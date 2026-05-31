package mysql

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/g8rswimmer/cogniflow/internal/store"
)

// Compile-time assertion that *WorkflowStore implements store.Store.
var _ store.Store = (*WorkflowStore)(nil)

// WorkflowStore implements store.Store for MySQL.
// Run, RAG, and trigger methods are stubbed until later milestones.
type WorkflowStore struct {
	db *sqlx.DB
}

// NewWorkflowStore creates a WorkflowStore backed by the given DB connection.
func NewWorkflowStore(db *sqlx.DB) *WorkflowStore {
	return &WorkflowStore{db: db}
}

// ---- Workflow CRUD -------------------------------------------------------

func (s *WorkflowStore) CreateWorkflow(ctx context.Context, w store.Workflow) (store.Workflow, error) {
	if w.ID == "" {
		w.ID = newUUID()
	}
	if w.TimeoutSeconds == 0 {
		w.TimeoutSeconds = 300
	}
	if w.Trigger.Kind == "" {
		w.Trigger.Kind = "manual"
	}

	now := time.Now().UTC()
	w.CreatedAt = now
	w.UpdatedAt = now

	triggerCfgBytes, err := json.Marshal(triggerExtra(w.Trigger))
	if err != nil {
		return store.Workflow{}, fmt.Errorf("workflow store: marshal trigger: %w", err)
	}

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return store.Workflow{}, fmt.Errorf("workflow store: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	_, err = tx.ExecContext(ctx,
		`INSERT INTO workflows (id, name, description, trigger_kind, trigger_config, timeout_seconds, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		w.ID, w.Name, w.Description, w.Trigger.Kind, string(triggerCfgBytes), w.TimeoutSeconds,
		w.CreatedAt, w.UpdatedAt,
	)
	if err != nil {
		return store.Workflow{}, fmt.Errorf("workflow store: insert workflow: %w", err)
	}

	if err := insertNodes(ctx, tx, w.ID, w.Nodes); err != nil {
		return store.Workflow{}, err
	}
	if err := insertEdges(ctx, tx, w.ID, w.Edges); err != nil {
		return store.Workflow{}, err
	}

	if err := tx.Commit(); err != nil {
		return store.Workflow{}, fmt.Errorf("workflow store: commit: %w", err)
	}

	return w, nil
}

func (s *WorkflowStore) GetWorkflow(ctx context.Context, id string) (store.Workflow, error) {
	var row dbWorkflow
	err := s.db.GetContext(ctx, &row,
		`SELECT id, name, COALESCE(description,'') AS description, trigger_kind,
		        trigger_config, timeout_seconds, created_at, updated_at
		 FROM workflows WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return store.Workflow{}, store.ErrNotFound
	}
	if err != nil {
		return store.Workflow{}, fmt.Errorf("workflow store: get workflow: %w", err)
	}

	w := store.Workflow{
		ID:             row.ID,
		Name:           row.Name,
		Description:    row.Description,
		TimeoutSeconds: row.TimeoutSeconds,
		Trigger:        store.Trigger{Kind: row.TriggerKind},
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
	if row.TriggerConfig != nil {
		var extra struct {
			CronExpr string `json:"cron_expr"`
		}
		_ = json.Unmarshal([]byte(*row.TriggerConfig), &extra)
		w.Trigger.CronExpr = extra.CronExpr
	}

	nodes, err := s.loadNodes(ctx, id)
	if err != nil {
		return store.Workflow{}, err
	}
	w.Nodes = nodes

	edges, err := s.loadEdges(ctx, id)
	if err != nil {
		return store.Workflow{}, err
	}
	w.Edges = edges

	return w, nil
}

func (s *WorkflowStore) ListWorkflows(ctx context.Context) ([]store.WorkflowSummary, error) {
	var rows []struct {
		ID             string    `db:"id"`
		Name           string    `db:"name"`
		Description    string    `db:"description"`
		TriggerKind    string    `db:"trigger_kind"`
		TimeoutSeconds int       `db:"timeout_seconds"`
		CreatedAt      time.Time `db:"created_at"`
		UpdatedAt      time.Time `db:"updated_at"`
	}
	if err := s.db.SelectContext(ctx, &rows,
		`SELECT id, name, COALESCE(description,'') AS description, trigger_kind,
		        timeout_seconds, created_at, updated_at
		 FROM workflows ORDER BY updated_at DESC`); err != nil {
		return nil, fmt.Errorf("workflow store: list workflows: %w", err)
	}

	summaries := make([]store.WorkflowSummary, 0, len(rows))
	for _, r := range rows {
		summaries = append(summaries, store.WorkflowSummary{
			ID:             r.ID,
			Name:           r.Name,
			Description:    r.Description,
			TriggerKind:    r.TriggerKind,
			TimeoutSeconds: r.TimeoutSeconds,
			CreatedAt:      r.CreatedAt,
			UpdatedAt:      r.UpdatedAt,
		})
	}
	return summaries, nil
}

func (s *WorkflowStore) UpdateWorkflow(ctx context.Context, w store.Workflow) (store.Workflow, error) {
	if w.Trigger.Kind == "" {
		w.Trigger.Kind = "manual"
	}

	triggerCfgBytes, err := json.Marshal(triggerExtra(w.Trigger))
	if err != nil {
		return store.Workflow{}, fmt.Errorf("workflow store: marshal trigger: %w", err)
	}

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return store.Workflow{}, fmt.Errorf("workflow store: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	res, err := tx.ExecContext(ctx,
		`UPDATE workflows SET name=?, description=?, trigger_kind=?, trigger_config=?, timeout_seconds=?
		 WHERE id=?`,
		w.Name, w.Description, w.Trigger.Kind, string(triggerCfgBytes), w.TimeoutSeconds, w.ID,
	)
	if err != nil {
		return store.Workflow{}, fmt.Errorf("workflow store: update workflow: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.Workflow{}, store.ErrNotFound
	}

	// Replace nodes and edges; CASCADE DELETE handles node_configs.
	if err := replaceNodesAndEdges(ctx, tx, w.ID, w.Nodes, w.Edges); err != nil {
		return store.Workflow{}, err
	}

	if err := tx.Commit(); err != nil {
		return store.Workflow{}, fmt.Errorf("workflow store: commit: %w", err)
	}

	// Fetch only timestamps to avoid a full round-trip; MySQL controls updated_at via ON UPDATE.
	var ts struct {
		CreatedAt time.Time `db:"created_at"`
		UpdatedAt time.Time `db:"updated_at"`
	}
	if err := s.db.GetContext(ctx, &ts,
		`SELECT created_at, updated_at FROM workflows WHERE id=?`, w.ID); err != nil {
		return store.Workflow{}, fmt.Errorf("workflow store: fetch timestamps: %w", err)
	}
	w.CreatedAt = ts.CreatedAt
	w.UpdatedAt = ts.UpdatedAt

	return w, nil
}

func (s *WorkflowStore) DeleteWorkflow(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM workflows WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("workflow store: delete workflow: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// ---- Runs (stub — implemented in M3) -------------------------------------

func (s *WorkflowStore) CreateRun(_ context.Context, _ store.Run) (store.Run, error) {
	return store.Run{}, fmt.Errorf("runs: not implemented until M3")
}
func (s *WorkflowStore) UpdateRunStatus(_ context.Context, _ string, _ store.RunStatus, _ map[string]any) error {
	return fmt.Errorf("runs: not implemented until M3")
}
func (s *WorkflowStore) GetRun(_ context.Context, _ string) (store.Run, error) {
	return store.Run{}, fmt.Errorf("runs: not implemented until M3")
}
func (s *WorkflowStore) ListRuns(_ context.Context, _ store.RunFilter) ([]store.Run, error) {
	return nil, fmt.Errorf("runs: not implemented until M3")
}

// ---- RAG (stub — implemented in M7) -------------------------------------

func (s *WorkflowStore) UpsertChunks(_ context.Context, _ []store.RAGChunk) error {
	return fmt.Errorf("rag: not implemented until M7")
}
func (s *WorkflowStore) SearchChunks(_ context.Context, _ []float32, _ int, _ string) ([]store.RAGChunkResult, error) {
	return nil, fmt.Errorf("rag: not implemented until M7")
}

// ---- Triggers (stub — implemented in M5) ---------------------------------

func (s *WorkflowStore) SaveTriggerConfig(_ context.Context, _ string, _ store.TriggerConfig) error {
	return fmt.Errorf("triggers: not implemented until M5")
}
func (s *WorkflowStore) GetTriggerConfig(_ context.Context, _ string) (store.TriggerConfig, error) {
	return store.TriggerConfig{}, fmt.Errorf("triggers: not implemented until M5")
}
func (s *WorkflowStore) ListTriggerConfigs(_ context.Context) ([]store.WorkflowTrigger, error) {
	return nil, fmt.Errorf("triggers: not implemented until M5")
}

// ---- internal helpers ----------------------------------------------------

type dbWorkflow struct {
	ID             string    `db:"id"`
	Name           string    `db:"name"`
	Description    string    `db:"description"`
	TriggerKind    string    `db:"trigger_kind"`
	TriggerConfig  *string   `db:"trigger_config"`
	TimeoutSeconds int       `db:"timeout_seconds"`
	CreatedAt      time.Time `db:"created_at"`
	UpdatedAt      time.Time `db:"updated_at"`
}

func replaceNodesAndEdges(ctx context.Context, tx *sqlx.Tx, workflowID string, nodes []store.WorkflowNode, edges []store.WorkflowEdge) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM workflow_nodes WHERE workflow_id=?`, workflowID); err != nil {
		return fmt.Errorf("workflow store: delete nodes: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM workflow_edges WHERE workflow_id=?`, workflowID); err != nil {
		return fmt.Errorf("workflow store: delete edges: %w", err)
	}
	if err := insertNodes(ctx, tx, workflowID, nodes); err != nil {
		return err
	}
	return insertEdges(ctx, tx, workflowID, edges)
}

func insertNodes(ctx context.Context, tx *sqlx.Tx, workflowID string, nodes []store.WorkflowNode) error {
	for _, n := range nodes {
		retryMax, retryMs := 0, 1000
		if n.RetryPolicy != nil {
			retryMax = n.RetryPolicy.MaxRetries
			retryMs = n.RetryPolicy.BackoffMs
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO workflow_nodes (id, workflow_id, type_id, label, position_x, position_y, retry_max, retry_backoff_ms)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			n.ID, workflowID, n.TypeID, n.Label,
			n.Position.X, n.Position.Y, retryMax, retryMs,
		)
		if err != nil {
			return fmt.Errorf("workflow store: insert node %q: %w", n.ID, err)
		}
		if err := insertConfigs(ctx, tx, n); err != nil {
			return err
		}
	}
	return nil
}

func insertConfigs(ctx context.Context, tx *sqlx.Tx, n store.WorkflowNode) error {
	for key, val := range n.Config {
		if n.SensitiveKeys[key] {
			ciphertext, ok := val.([]byte)
			if !ok {
				return fmt.Errorf("workflow store: sensitive value for %q is not []byte", key)
			}
			_, err := tx.ExecContext(ctx,
				`INSERT INTO node_configs (node_id, config_key, plain_value, encrypted_value, is_sensitive)
				 VALUES (?, ?, NULL, ?, 1)`,
				n.ID, key, ciphertext,
			)
			if err != nil {
				return fmt.Errorf("workflow store: insert encrypted config %q: %w", key, err)
			}
		} else {
			encoded, err := json.Marshal(val)
			if err != nil {
				return fmt.Errorf("workflow store: marshal config %q: %w", key, err)
			}
			_, err = tx.ExecContext(ctx,
				`INSERT INTO node_configs (node_id, config_key, plain_value, encrypted_value, is_sensitive)
				 VALUES (?, ?, ?, NULL, 0)`,
				n.ID, key, string(encoded),
			)
			if err != nil {
				return fmt.Errorf("workflow store: insert plain config %q: %w", key, err)
			}
		}
	}
	return nil
}

func insertEdges(ctx context.Context, tx *sqlx.Tx, workflowID string, edges []store.WorkflowEdge) error {
	for _, e := range edges {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO workflow_edges (id, workflow_id, source_id, target_id, branch_label)
			 VALUES (?, ?, ?, ?, ?)`,
			e.ID, workflowID, e.SourceID, e.TargetID, e.BranchLabel,
		)
		if err != nil {
			return fmt.Errorf("workflow store: insert edge %q: %w", e.ID, err)
		}
	}
	return nil
}

func (s *WorkflowStore) loadNodes(ctx context.Context, workflowID string) ([]store.WorkflowNode, error) {
	var rows []struct {
		ID             string  `db:"id"`
		TypeID         string  `db:"type_id"`
		Label          string  `db:"label"`
		PositionX      float64 `db:"position_x"`
		PositionY      float64 `db:"position_y"`
		RetryMax       int     `db:"retry_max"`
		RetryBackoffMs int     `db:"retry_backoff_ms"`
	}
	if err := s.db.SelectContext(ctx, &rows,
		`SELECT id, type_id, COALESCE(label,'') AS label, position_x, position_y, retry_max, retry_backoff_ms
		 FROM workflow_nodes WHERE workflow_id=? ORDER BY id`, workflowID); err != nil {
		return nil, fmt.Errorf("workflow store: load nodes: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}

	// Collect node IDs for a single batch config query.
	nodeIDs := make([]string, len(rows))
	for i, r := range rows {
		nodeIDs[i] = r.ID
	}
	configs, sensitiveKeys, err := s.loadConfigs(ctx, nodeIDs)
	if err != nil {
		return nil, err
	}

	nodes := make([]store.WorkflowNode, 0, len(rows))
	for _, r := range rows {
		n := store.WorkflowNode{
			ID:            r.ID,
			TypeID:        r.TypeID,
			Label:         r.Label,
			Position:      store.NodePosition{X: r.PositionX, Y: r.PositionY},
			Config:        configs[r.ID],
			SensitiveKeys: sensitiveKeys[r.ID],
		}
		if r.RetryMax > 0 {
			n.RetryPolicy = &store.RetryPolicy{
				MaxRetries: r.RetryMax,
				BackoffMs:  r.RetryBackoffMs,
			}
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}

// loadConfigs fetches all node_configs rows for the given node IDs in a single
// query, eliminating the N+1 pattern in loadNodes.
func (s *WorkflowStore) loadConfigs(ctx context.Context, nodeIDs []string) (map[string]map[string]any, map[string]map[string]bool, error) {
	query, args, err := sqlx.In(
		`SELECT node_id, config_key, plain_value, encrypted_value, is_sensitive
		 FROM node_configs WHERE node_id IN (?)`, nodeIDs,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("workflow store: build config query: %w", err)
	}
	query = s.db.Rebind(query)

	var rows []struct {
		NodeID      string `db:"node_id"`
		Key         string `db:"config_key"`
		PlainValue  []byte `db:"plain_value"`
		EncValue    []byte `db:"encrypted_value"`
		IsSensitive bool   `db:"is_sensitive"`
	}
	if err := s.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, nil, fmt.Errorf("workflow store: load configs: %w", err)
	}

	configs := make(map[string]map[string]any, len(nodeIDs))
	sensitives := make(map[string]map[string]bool, len(nodeIDs))
	for _, id := range nodeIDs {
		configs[id] = make(map[string]any)
		sensitives[id] = make(map[string]bool)
	}

	for _, r := range rows {
		if r.IsSensitive {
			configs[r.NodeID][r.Key] = r.EncValue
			sensitives[r.NodeID][r.Key] = true
		} else {
			var val any
			if len(r.PlainValue) > 0 {
				if err := json.Unmarshal(r.PlainValue, &val); err != nil {
					val = string(r.PlainValue)
				}
			}
			configs[r.NodeID][r.Key] = val
			sensitives[r.NodeID][r.Key] = false
		}
	}
	return configs, sensitives, nil
}

func (s *WorkflowStore) loadEdges(ctx context.Context, workflowID string) ([]store.WorkflowEdge, error) {
	var rows []struct {
		ID          string  `db:"id"`
		SourceID    string  `db:"source_id"`
		TargetID    string  `db:"target_id"`
		BranchLabel *string `db:"branch_label"`
	}
	if err := s.db.SelectContext(ctx, &rows,
		`SELECT id, source_id, target_id, branch_label
		 FROM workflow_edges WHERE workflow_id=? ORDER BY id`, workflowID); err != nil {
		return nil, fmt.Errorf("workflow store: load edges: %w", err)
	}

	edges := make([]store.WorkflowEdge, 0, len(rows))
	for _, r := range rows {
		edges = append(edges, store.WorkflowEdge{
			ID:          r.ID,
			SourceID:    r.SourceID,
			TargetID:    r.TargetID,
			BranchLabel: r.BranchLabel,
		})
	}
	return edges, nil
}

func triggerExtra(t store.Trigger) map[string]any {
	m := map[string]any{}
	if t.CronExpr != "" {
		m["cron_expr"] = t.CronExpr
	}
	return m
}

func newUUID() string {
	var u [16]byte
	if _, err := io.ReadFull(rand.Reader, u[:]); err != nil {
		panic(fmt.Sprintf("uuid: read random: %v", err))
	}
	u[6] = (u[6] & 0x0f) | 0x40
	u[8] = (u[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		u[0:4], u[4:6], u[6:8], u[8:10], u[10:])
}
