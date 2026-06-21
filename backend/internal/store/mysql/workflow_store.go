package mysql

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
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

	_, err = tx.NamedExecContext(ctx,
		`INSERT INTO workflows (id, name, description, trigger_kind, trigger_config, initial_data_schema, timeout_seconds, created_at, updated_at)
		 VALUES (:id, :name, :description, :trigger_kind, :trigger_config, :initial_data_schema, :timeout_seconds, :created_at, :updated_at)`,
		workflowWriteRow{
			ID:                w.ID,
			Name:              w.Name,
			Description:       w.Description,
			TriggerKind:       w.Trigger.Kind,
			TriggerConfig:     string(triggerCfgBytes),
			InitialDataSchema: rawMessageToPtr(w.InitialDataSchema),
			TimeoutSeconds:    w.TimeoutSeconds,
			CreatedAt:         w.CreatedAt,
			UpdatedAt:         w.UpdatedAt,
		},
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

func (s *WorkflowStore) GetWorkflowSchema(ctx context.Context, id string) (json.RawMessage, error) {
	var row struct {
		InitialDataSchema *string `db:"initial_data_schema"`
	}
	err := s.db.GetContext(ctx, &row,
		`SELECT initial_data_schema FROM workflows WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("workflow store: get workflow schema: %w", err)
	}
	return ptrToRawMessage(row.InitialDataSchema), nil
}

func (s *WorkflowStore) GetWorkflow(ctx context.Context, id string) (store.Workflow, error) {
	var row dbWorkflow
	err := s.db.GetContext(ctx, &row,
		`SELECT id, name, COALESCE(description,'') AS description, trigger_kind,
		        trigger_config, initial_data_schema, timeout_seconds, created_at, updated_at
		 FROM workflows WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return store.Workflow{}, store.ErrNotFound
	}
	if err != nil {
		return store.Workflow{}, fmt.Errorf("workflow store: get workflow: %w", err)
	}

	tc := unmarshalTriggerConfig(row.TriggerKind, row.TriggerConfig)
	w := store.Workflow{
		ID:                row.ID,
		Name:              row.Name,
		Description:       row.Description,
		TimeoutSeconds:    row.TimeoutSeconds,
		Trigger:           store.Trigger(tc),
		InitialDataSchema: ptrToRawMessage(row.InitialDataSchema),
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
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
		NodeCount      int       `db:"node_count"`
		CreatedAt      time.Time `db:"created_at"`
		UpdatedAt      time.Time `db:"updated_at"`
	}
	if err := s.db.SelectContext(ctx, &rows,
		`SELECT w.id, w.name, COALESCE(w.description,'') AS description, w.trigger_kind,
		        w.timeout_seconds, w.created_at, w.updated_at,
		        COUNT(n.id) AS node_count
		 FROM workflows w
		 LEFT JOIN workflow_nodes n ON n.workflow_id = w.id
		 GROUP BY w.id
		 ORDER BY w.updated_at DESC`); err != nil {
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
			NodeCount:      r.NodeCount,
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

	res, err := tx.NamedExecContext(ctx,
		`UPDATE workflows
		 SET name=:name, description=:description, trigger_kind=:trigger_kind,
		     trigger_config=:trigger_config, initial_data_schema=:initial_data_schema,
		     timeout_seconds=:timeout_seconds
		 WHERE id=:id`,
		workflowWriteRow{
			ID:                w.ID,
			Name:              w.Name,
			Description:       w.Description,
			TriggerKind:       w.Trigger.Kind,
			TriggerConfig:     string(triggerCfgBytes),
			InitialDataSchema: rawMessageToPtr(w.InitialDataSchema),
			TimeoutSeconds:    w.TimeoutSeconds,
		},
	)
	if err != nil {
		return store.Workflow{}, fmt.Errorf("workflow store: update workflow: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.Workflow{}, store.ErrNotFound
	}

	if err := replaceNodesAndEdges(ctx, tx, w.ID, w.Nodes, w.Edges); err != nil {
		return store.Workflow{}, err
	}

	// Read MySQL-managed timestamps inside the transaction so the value is
	// guaranteed visible and no concurrent DeleteWorkflow can delete the row
	// between our Commit and a post-commit SELECT.
	var ts struct {
		CreatedAt time.Time `db:"created_at"`
		UpdatedAt time.Time `db:"updated_at"`
	}
	if err := tx.GetContext(ctx, &ts,
		`SELECT created_at, updated_at FROM workflows WHERE id=?`, w.ID); err != nil {
		return store.Workflow{}, fmt.Errorf("workflow store: fetch timestamps: %w", err)
	}
	w.CreatedAt = ts.CreatedAt
	w.UpdatedAt = ts.UpdatedAt

	if err := tx.Commit(); err != nil {
		return store.Workflow{}, fmt.Errorf("workflow store: commit: %w", err)
	}

	return w, nil
}

func (s *WorkflowStore) DeleteWorkflow(ctx context.Context, id string) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("workflow store: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Explicitly delete all child rows in dependency order.
	// node_configs must go before workflow_nodes since they reference node IDs.
	// TODO: also delete rag_documents/rag_chunks for this workflow. Currently,
	// RAG data is not workflow-scoped at the schema level (document_id may be a
	// template that can only be resolved at run time). A future migration should
	// add workflow_id to rag_documents to enable deterministic cleanup here.
	if err := deleteNodeConfigs(ctx, tx, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM workflow_nodes WHERE workflow_id = ?`, id); err != nil {
		return fmt.Errorf("workflow store: delete workflow nodes: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM workflow_edges WHERE workflow_id = ?`, id); err != nil {
		return fmt.Errorf("workflow store: delete workflow edges: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM runs WHERE workflow_id = ?`, id); err != nil {
		return fmt.Errorf("workflow store: delete runs: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM workflow_versions WHERE workflow_id = ?`, id); err != nil {
		return fmt.Errorf("workflow store: delete workflow versions: %w", err)
	}

	res, err := tx.ExecContext(ctx, `DELETE FROM workflows WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("workflow store: delete workflow: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("workflow store: commit: %w", err)
	}
	return nil
}


// ---- Triggers ------------------------------------------------------------

// SaveTriggerConfig updates the trigger_kind and trigger_config columns for the
// given workflow. It uses the same columns as CreateWorkflow/UpdateWorkflow so
// no additional table or migration is needed.
func (s *WorkflowStore) SaveTriggerConfig(ctx context.Context, workflowID string, cfg store.TriggerConfig) error {
	extra := map[string]any{}
	if cfg.CronExpr != "" {
		extra["cron_expr"] = cfg.CronExpr
	}
	b, err := json.Marshal(extra)
	if err != nil {
		return fmt.Errorf("trigger store: marshal config: %w", err)
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE workflows SET trigger_kind=?, trigger_config=? WHERE id=?`,
		cfg.Kind, string(b), workflowID,
	)
	if err != nil {
		return fmt.Errorf("trigger store: save trigger config: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// GetTriggerConfig returns the trigger configuration for the given workflow.
func (s *WorkflowStore) GetTriggerConfig(ctx context.Context, workflowID string) (store.TriggerConfig, error) {
	var row struct {
		TriggerKind   string  `db:"trigger_kind"`
		TriggerConfig *string `db:"trigger_config"`
	}
	err := s.db.GetContext(ctx, &row,
		`SELECT trigger_kind, trigger_config FROM workflows WHERE id=?`, workflowID)
	if errors.Is(err, sql.ErrNoRows) {
		return store.TriggerConfig{}, store.ErrNotFound
	}
	if err != nil {
		return store.TriggerConfig{}, fmt.Errorf("trigger store: get trigger config: %w", err)
	}
	return unmarshalTriggerConfig(row.TriggerKind, row.TriggerConfig), nil
}

// ListTriggerConfigs returns trigger configurations for all workflows that have
// a non-manual trigger (webhook or cron). Used by TriggerManager.LoadAll.
func (s *WorkflowStore) ListTriggerConfigs(ctx context.Context) ([]store.WorkflowTrigger, error) {
	var rows []struct {
		ID            string  `db:"id"`
		TriggerKind   string  `db:"trigger_kind"`
		TriggerConfig *string `db:"trigger_config"`
	}
	if err := s.db.SelectContext(ctx, &rows,
		`SELECT id, trigger_kind, trigger_config FROM workflows WHERE trigger_kind != 'manual'`,
	); err != nil {
		return nil, fmt.Errorf("trigger store: list trigger configs: %w", err)
	}
	result := make([]store.WorkflowTrigger, 0, len(rows))
	for _, r := range rows {
		result = append(result, store.WorkflowTrigger{
			WorkflowID: r.ID,
			Config:     unmarshalTriggerConfig(r.TriggerKind, r.TriggerConfig),
		})
	}
	return result, nil
}

// unmarshalTriggerConfig builds a TriggerConfig from the raw DB columns,
// extracting any kind-specific fields from the JSON trigger_config blob.
func unmarshalTriggerConfig(kind string, raw *string) store.TriggerConfig {
	cfg := store.TriggerConfig{Kind: kind}
	if raw != nil {
		var extra struct {
			CronExpr string `json:"cron_expr"`
		}
		_ = json.Unmarshal([]byte(*raw), &extra)
		cfg.CronExpr = extra.CronExpr
	}
	return cfg
}

// rawMessageToPtr converts a json.RawMessage to a *string for nullable DB columns.
// Returns nil (SQL NULL) when the message is empty or the JSON null literal,
// so that "no schema defined" is always stored as SQL NULL rather than the string "null".
func rawMessageToPtr(msg json.RawMessage) *string {
	if len(msg) == 0 || string(msg) == "null" {
		return nil
	}
	s := string(msg)
	return &s
}

// ptrToRawMessage converts a nullable *string DB column back to json.RawMessage.
func ptrToRawMessage(s *string) json.RawMessage {
	if s == nil {
		return nil
	}
	return json.RawMessage(*s)
}

// ---- internal helpers ----------------------------------------------------

// defaultRetryBackoffMs is the backoff stored when a node has no RetryPolicy.
// loadNodes uses this sentinel to distinguish a saved nil policy from an explicit
// RetryPolicy{MaxRetries:0, BackoffMs:defaultRetryBackoffMs}.
const defaultRetryBackoffMs = 1000

// dbWorkflow is the read-side row struct for SELECT queries against the workflows table.
type dbWorkflow struct {
	ID                string    `db:"id"`
	Name              string    `db:"name"`
	Description       string    `db:"description"`
	TriggerKind       string    `db:"trigger_kind"`
	TriggerConfig     *string   `db:"trigger_config"`      // nullable on read
	InitialDataSchema *string   `db:"initial_data_schema"` // nullable on read
	TimeoutSeconds    int       `db:"timeout_seconds"`
	CreatedAt         time.Time `db:"created_at"`
	UpdatedAt         time.Time `db:"updated_at"`
}

// workflowWriteRow is the write-side row struct for INSERT/UPDATE named queries.
type workflowWriteRow struct {
	ID                string    `db:"id"`
	Name              string    `db:"name"`
	Description       string    `db:"description"`
	TriggerKind       string    `db:"trigger_kind"`
	TriggerConfig     string    `db:"trigger_config"`
	InitialDataSchema *string   `db:"initial_data_schema"` // nil → NULL
	TimeoutSeconds    int       `db:"timeout_seconds"`
	CreatedAt         time.Time `db:"created_at"`
	UpdatedAt         time.Time `db:"updated_at"`
}

// nodeWriteRow is the write-side row struct for INSERT into workflow_nodes.
type nodeWriteRow struct {
	ID             string  `db:"id"`
	WorkflowID     string  `db:"workflow_id"`
	TypeID         string  `db:"type_id"`
	Label          string  `db:"label"`
	PositionX      float64 `db:"position_x"`
	PositionY      float64 `db:"position_y"`
	RetryMax       int     `db:"retry_max"`
	RetryBackoffMs int     `db:"retry_backoff_ms"`
	OutputParsers  *string `db:"output_parsers"` // JSON blob; NULL when no parsers defined
}

// edgeWriteRow is the write-side row struct for INSERT into workflow_edges.
type edgeWriteRow struct {
	ID          string  `db:"id"`
	WorkflowID  string  `db:"workflow_id"`
	SourceID    string  `db:"source_id"`
	TargetID    string  `db:"target_id"`
	BranchLabel *string `db:"branch_label"`
	IsLoopBack  bool    `db:"is_loop_back"`
}

// configWriteRow is the write-side row struct for INSERT into node_configs.
// PlainValue and EncValue are mutually exclusive: one is nil depending on IsSensitive.
type configWriteRow struct {
	WorkflowID  string  `db:"workflow_id"`
	NodeID      string  `db:"node_id"`
	Key         string  `db:"config_key"`
	PlainValue  *string `db:"plain_value"`
	EncValue    []byte  `db:"encrypted_value"`
	IsSensitive bool    `db:"is_sensitive"`
}

func replaceNodesAndEdges(ctx context.Context, tx *sqlx.Tx, workflowID string, nodes []store.WorkflowNode, edges []store.WorkflowEdge) error {
	// Delete node_configs before workflow_nodes; without FK cascade this must be explicit.
	if err := deleteNodeConfigs(ctx, tx, workflowID); err != nil {
		return err
	}
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
		retryMax, retryMs := 0, defaultRetryBackoffMs
		if n.RetryPolicy != nil {
			retryMax = n.RetryPolicy.MaxRetries
			retryMs = n.RetryPolicy.BackoffMs
		}

		var parsersJSON *string
		if len(n.OutputParsers) > 0 {
			b, err := json.Marshal(n.OutputParsers)
			if err != nil {
				return fmt.Errorf("workflow store: marshal output_parsers for node %q: %w", n.ID, err)
			}
			s := string(b)
			parsersJSON = &s
		}

		_, err := tx.NamedExecContext(ctx,
			`INSERT INTO workflow_nodes (id, workflow_id, type_id, label, position_x, position_y, retry_max, retry_backoff_ms, output_parsers)
			 VALUES (:id, :workflow_id, :type_id, :label, :position_x, :position_y, :retry_max, :retry_backoff_ms, :output_parsers)`,
			nodeWriteRow{
				ID:             n.ID,
				WorkflowID:     workflowID,
				TypeID:         n.TypeID,
				Label:          n.Label,
				PositionX:      n.Position.X,
				PositionY:      n.Position.Y,
				RetryMax:       retryMax,
				RetryBackoffMs: retryMs,
				OutputParsers:  parsersJSON,
			},
		)
		if err != nil {
			return fmt.Errorf("workflow store: insert node %q: %w", n.ID, err)
		}
		if err := insertConfigs(ctx, tx, workflowID, n); err != nil {
			return err
		}
	}
	return nil
}

func insertConfigs(ctx context.Context, tx *sqlx.Tx, workflowID string, n store.WorkflowNode) error {
	for key, val := range n.Config {
		row := configWriteRow{WorkflowID: workflowID, NodeID: n.ID, Key: key}
		if n.SensitiveKeys[key] {
			ciphertext, ok := val.([]byte)
			if !ok {
				return fmt.Errorf("workflow store: sensitive value for %q is not []byte", key)
			}
			row.EncValue = ciphertext
			row.IsSensitive = true
		} else {
			encoded, err := json.Marshal(val)
			if err != nil {
				return fmt.Errorf("workflow store: marshal config %q: %w", key, err)
			}
			s := string(encoded)
			row.PlainValue = &s
		}
		_, err := tx.NamedExecContext(ctx,
			`INSERT INTO node_configs (workflow_id, node_id, config_key, plain_value, encrypted_value, is_sensitive)
			 VALUES (:workflow_id, :node_id, :config_key, :plain_value, :encrypted_value, :is_sensitive)`,
			row,
		)
		if err != nil {
			return fmt.Errorf("workflow store: insert config %q: %w", key, err)
		}
	}
	return nil
}

func insertEdges(ctx context.Context, tx *sqlx.Tx, workflowID string, edges []store.WorkflowEdge) error {
	for _, e := range edges {
		_, err := tx.NamedExecContext(ctx,
			`INSERT INTO workflow_edges (id, workflow_id, source_id, target_id, branch_label, is_loop_back)
			 VALUES (:id, :workflow_id, :source_id, :target_id, :branch_label, :is_loop_back)`,
			edgeWriteRow{
				ID:          e.ID,
				WorkflowID:  workflowID,
				SourceID:    e.SourceID,
				TargetID:    e.TargetID,
				BranchLabel: e.BranchLabel,
				IsLoopBack:  e.IsLoopBack,
			},
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
		OutputParsers  *string `db:"output_parsers"`
	}
	if err := s.db.SelectContext(ctx, &rows,
		`SELECT id, type_id, COALESCE(label,'') AS label, position_x, position_y,
		        retry_max, retry_backoff_ms, output_parsers
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
	configs, sensitiveKeys, err := s.loadConfigs(ctx, workflowID, nodeIDs)
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
		// Reconstruct RetryPolicy when either MaxRetries is positive OR the backoff
		// differs from the nil-policy default. This preserves RetryPolicy{MaxRetries:0,
		// BackoffMs:N} for N != defaultRetryBackoffMs, which would otherwise be silently
		// dropped because RetryMax == 0.
		if r.RetryMax > 0 || r.RetryBackoffMs != defaultRetryBackoffMs {
			n.RetryPolicy = &store.RetryPolicy{
				MaxRetries: r.RetryMax,
				BackoffMs:  r.RetryBackoffMs,
			}
		}
		if r.OutputParsers != nil && *r.OutputParsers != "" {
			var parsers map[string]store.OutputParser
			if err := json.Unmarshal([]byte(*r.OutputParsers), &parsers); err != nil {
				slog.Warn("workflow store: ignoring malformed output_parsers for node",
					"node_id", r.ID, "error", err)
			} else {
				n.OutputParsers = parsers
			}
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}

// loadConfigs fetches all node_configs rows for the given workflow and node IDs
// in a single query, eliminating the N+1 pattern in loadNodes.
func (s *WorkflowStore) loadConfigs(ctx context.Context, workflowID string, nodeIDs []string) (map[string]map[string]any, map[string]map[string]bool, error) {
	query, args, err := sqlx.In(
		`SELECT node_id, config_key, plain_value, encrypted_value, is_sensitive
		 FROM node_configs WHERE workflow_id = ? AND node_id IN (?)`, workflowID, nodeIDs,
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
		IsLoopBack  bool    `db:"is_loop_back"`
	}
	if err := s.db.SelectContext(ctx, &rows,
		`SELECT id, source_id, target_id, branch_label, is_loop_back
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
			IsLoopBack:  r.IsLoopBack,
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

// deleteNodeConfigs removes all node_configs rows for the given workflow.
// Must be called inside a transaction before workflow_nodes are deleted.
func deleteNodeConfigs(ctx context.Context, tx *sqlx.Tx, workflowID string) error {
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM node_configs WHERE workflow_id = ?`,
		workflowID,
	); err != nil {
		return fmt.Errorf("workflow store: delete node configs: %w", err)
	}
	return nil
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
