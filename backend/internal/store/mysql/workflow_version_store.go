package mysql

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/g8rswimmer/cogniflow/internal/store"
)

// versionNodeSnapshot is the per-node structure stored in a version definition.
// It differs from store.WorkflowNode by explicitly including SensitiveKeys
// (which has json:"-" on the store type) so encrypted config []byte values
// survive json.Marshal as base64 strings and can be decoded on restore.
type versionNodeSnapshot struct {
	ID            string                       `json:"id"`
	TypeID        string                       `json:"type_id"`
	Label         string                       `json:"label,omitempty"`
	Position      store.NodePosition           `json:"position"`
	Config        map[string]any               `json:"config,omitempty"`
	SensitiveKeys map[string]bool              `json:"sensitive_keys"`
	RetryPolicy   *store.RetryPolicy           `json:"retry_policy,omitempty"`
	OutputParsers map[string]store.OutputParser `json:"output_parsers,omitempty"`
}

// versionSnapshot is the JSON blob stored in workflow_versions.definition.
type versionSnapshot struct {
	ID                string                `json:"id"`
	Name              string                `json:"name"`
	Description       string                `json:"description,omitempty"`
	Trigger           store.Trigger         `json:"trigger"`
	TimeoutSeconds    int                   `json:"timeout_seconds"`
	Nodes             []versionNodeSnapshot `json:"nodes"`
	Edges             []store.WorkflowEdge  `json:"edges"`
	InitialDataSchema json.RawMessage       `json:"initial_data_schema,omitempty"`
}

// workflowToSnapshot converts a store.Workflow to a versionSnapshot.
// Sensitive Config values are []byte ciphertexts; encoding/json automatically
// base64-encodes []byte, preserving the encrypted bytes across JSON round-trips.
func workflowToSnapshot(w store.Workflow) versionSnapshot {
	nodes := make([]versionNodeSnapshot, len(w.Nodes))
	for i, n := range w.Nodes {
		nodes[i] = versionNodeSnapshot{
			ID:            n.ID,
			TypeID:        n.TypeID,
			Label:         n.Label,
			Position:      n.Position,
			Config:        n.Config,
			SensitiveKeys: n.SensitiveKeys,
			RetryPolicy:   n.RetryPolicy,
			OutputParsers: n.OutputParsers,
		}
	}
	return versionSnapshot{
		ID:                w.ID,
		Name:              w.Name,
		Description:       w.Description,
		Trigger:           w.Trigger,
		TimeoutSeconds:    w.TimeoutSeconds,
		Nodes:             nodes,
		Edges:             w.Edges,
		InitialDataSchema: w.InitialDataSchema,
	}
}

// snapshotToWorkflow reconstructs a store.Workflow from a parsed versionSnapshot.
// For sensitive config fields, the base64 string produced by json.Unmarshal into
// map[string]any is decoded back to []byte so insertConfigs routes it to the
// encrypted_value column, preserving the original ciphertext.
func snapshotToWorkflow(snap versionSnapshot) (store.Workflow, error) {
	nodes := make([]store.WorkflowNode, len(snap.Nodes))
	for i, sn := range snap.Nodes {
		cfg := make(map[string]any, len(sn.Config))
		for k, v := range sn.Config {
			if sn.SensitiveKeys[k] {
				strVal, ok := v.(string)
				if !ok {
					return store.Workflow{}, fmt.Errorf(
						"workflow version store: sensitive config %q is not a string after JSON decode", k)
				}
				decoded, err := base64.StdEncoding.DecodeString(strVal)
				if err != nil {
					return store.Workflow{}, fmt.Errorf(
						"workflow version store: base64 decode sensitive config %q: %w", k, err)
				}
				cfg[k] = decoded // []byte — ready for insertConfigs encrypted_value path
			} else {
				cfg[k] = v
			}
		}
		nodes[i] = store.WorkflowNode{
			ID:            sn.ID,
			TypeID:        sn.TypeID,
			Label:         sn.Label,
			Position:      sn.Position,
			Config:        cfg,
			SensitiveKeys: sn.SensitiveKeys,
			RetryPolicy:   sn.RetryPolicy,
			OutputParsers: sn.OutputParsers,
		}
	}
	return store.Workflow{
		ID:                snap.ID,
		Name:              snap.Name,
		Description:       snap.Description,
		Trigger:           snap.Trigger,
		TimeoutSeconds:    snap.TimeoutSeconds,
		Nodes:             nodes,
		Edges:             snap.Edges,
		InitialDataSchema: snap.InitialDataSchema,
	}, nil
}

// ---- Store methods ----------------------------------------------------------

// CreateWorkflowVersion snapshots the supplied workflow (which must have []byte
// ciphertexts in Config for any sensitive fields) as the next version number.
// Called by ConfigVault.UpdateWorkflow after the inner store write succeeds,
// before decryption, so the snapshot captures the encrypted-at-rest representation.
func (s *WorkflowStore) CreateWorkflowVersion(ctx context.Context, w store.Workflow) error {
	snap := workflowToSnapshot(w)
	defJSON, err := json.Marshal(snap)
	if err != nil {
		return fmt.Errorf("workflow version store: marshal definition: %w", err)
	}

	// Wrap SELECT MAX + INSERT in a transaction so the version_number is assigned
	// atomically. The UNIQUE constraint on (workflow_id, version_number) provides a
	// second layer of protection: if two concurrent saves race through this transaction,
	// the losing INSERT fails with a unique-key error rather than silently inserting a
	// duplicate version number.
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("workflow version store: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var maxVer sql.NullInt32
	if err := tx.GetContext(ctx, &maxVer,
		`SELECT MAX(version_number) FROM workflow_versions WHERE workflow_id=?`, w.ID); err != nil {
		return fmt.Errorf("workflow version store: get max version: %w", err)
	}
	nextVer := 1
	if maxVer.Valid {
		nextVer = int(maxVer.Int32) + 1
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO workflow_versions (id, workflow_id, version_number, node_count, definition, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		newUUID(), w.ID, nextVer, len(w.Nodes), string(defJSON), time.Now().UTC(),
	); err != nil {
		return fmt.Errorf("workflow version store: insert version: %w", err)
	}
	return tx.Commit()
}

// ListWorkflowVersions returns all version summaries for a workflow, newest first.
func (s *WorkflowStore) ListWorkflowVersions(ctx context.Context, workflowID string) ([]store.WorkflowVersionSummary, error) {
	var rows []struct {
		ID            string    `db:"id"`
		WorkflowID    string    `db:"workflow_id"`
		VersionNumber int       `db:"version_number"`
		NodeCount     int       `db:"node_count"`
		CreatedAt     time.Time `db:"created_at"`
	}
	if err := s.db.SelectContext(ctx, &rows,
		`SELECT id, workflow_id, version_number, node_count, created_at
		 FROM workflow_versions WHERE workflow_id=? ORDER BY version_number DESC`, workflowID); err != nil {
		return nil, fmt.Errorf("workflow version store: list versions: %w", err)
	}
	summaries := make([]store.WorkflowVersionSummary, len(rows))
	for i, r := range rows {
		summaries[i] = store.WorkflowVersionSummary{
			ID:            r.ID,
			WorkflowID:    r.WorkflowID,
			VersionNumber: r.VersionNumber,
			NodeCount:     r.NodeCount,
			CreatedAt:     r.CreatedAt,
		}
	}
	return summaries, nil
}

// GetWorkflowVersion returns the full version definition. Sensitive Config values
// are returned as []byte ciphertexts; ConfigVault.GetWorkflowVersion decrypts them.
func (s *WorkflowStore) GetWorkflowVersion(ctx context.Context, workflowID string, versionNum int) (store.WorkflowVersion, error) {
	var row struct {
		ID            string    `db:"id"`
		WorkflowID    string    `db:"workflow_id"`
		VersionNumber int       `db:"version_number"`
		Definition    string    `db:"definition"`
		CreatedAt     time.Time `db:"created_at"`
	}
	err := s.db.GetContext(ctx, &row,
		`SELECT id, workflow_id, version_number, definition, created_at
		 FROM workflow_versions WHERE workflow_id=? AND version_number=?`,
		workflowID, versionNum)
	if errors.Is(err, sql.ErrNoRows) {
		return store.WorkflowVersion{}, store.ErrNotFound
	}
	if err != nil {
		return store.WorkflowVersion{}, fmt.Errorf("workflow version store: get version: %w", err)
	}

	var snap versionSnapshot
	if err := json.Unmarshal([]byte(row.Definition), &snap); err != nil {
		return store.WorkflowVersion{}, fmt.Errorf("workflow version store: parse definition: %w", err)
	}
	wf, err := snapshotToWorkflow(snap)
	if err != nil {
		return store.WorkflowVersion{}, err
	}
	return store.WorkflowVersion{
		ID:            row.ID,
		WorkflowID:    row.WorkflowID,
		VersionNumber: row.VersionNumber,
		Definition:    wf,
		CreatedAt:     row.CreatedAt,
	}, nil
}

// DeleteWorkflowVersions removes all version records for a workflow.
// Called inside DeleteWorkflow before the workflow row is deleted.
func (s *WorkflowStore) DeleteWorkflowVersions(ctx context.Context, workflowID string) error {
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM workflow_versions WHERE workflow_id=?`, workflowID); err != nil {
		return fmt.Errorf("workflow version store: delete versions: %w", err)
	}
	return nil
}

// RestoreWorkflowVersion replaces the current workflow state with a prior snapshot.
// It reconstructs the nodes (decoding base64 ciphertexts back to []byte) and
// writes them via replaceNodesAndEdges so the encrypted_value column is populated
// correctly — identical to a normal UpdateWorkflow write. After committing,
// the restored state is snapshotted as the next version number (best-effort).
func (s *WorkflowStore) RestoreWorkflowVersion(ctx context.Context, workflowID string, versionNum int) (store.Workflow, error) {
	var defStr string
	err := s.db.GetContext(ctx, &defStr,
		`SELECT definition FROM workflow_versions WHERE workflow_id=? AND version_number=?`,
		workflowID, versionNum)
	if errors.Is(err, sql.ErrNoRows) {
		return store.Workflow{}, store.ErrNotFound
	}
	if err != nil {
		return store.Workflow{}, fmt.Errorf("workflow version store: get version for restore: %w", err)
	}

	var snap versionSnapshot
	if err := json.Unmarshal([]byte(defStr), &snap); err != nil {
		return store.Workflow{}, fmt.Errorf("workflow version store: parse definition for restore: %w", err)
	}
	wf, err := snapshotToWorkflow(snap)
	if err != nil {
		return store.Workflow{}, err
	}
	wf.ID = workflowID

	triggerCfgBytes, err := json.Marshal(triggerExtra(wf.Trigger))
	if err != nil {
		return store.Workflow{}, fmt.Errorf("workflow version store: marshal trigger: %w", err)
	}

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return store.Workflow{}, fmt.Errorf("workflow version store: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if err := replaceNodesAndEdges(ctx, tx, workflowID, wf.Nodes, wf.Edges); err != nil {
		return store.Workflow{}, err
	}

	now := time.Now().UTC()
	res, err := tx.ExecContext(ctx,
		`UPDATE workflows SET name=?, description=?, trigger_kind=?, trigger_config=?, timeout_seconds=?, initial_data_schema=?, updated_at=? WHERE id=?`,
		wf.Name, wf.Description, wf.Trigger.Kind, string(triggerCfgBytes),
		wf.TimeoutSeconds, rawMessageToPtr(wf.InitialDataSchema), now, workflowID,
	)
	if err != nil {
		return store.Workflow{}, fmt.Errorf("workflow version store: update workflow: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.Workflow{}, fmt.Errorf("workflow version store: workflow %q: %w", workflowID, store.ErrNotFound)
	}

	if err := tx.Commit(); err != nil {
		return store.Workflow{}, fmt.Errorf("workflow version store: commit restore: %w", err)
	}

	wf.UpdatedAt = now

	// Re-read the workflow's created_at from the DB so the restore response carries
	// the correct original creation timestamp (it is not stored in the version snapshot).
	var createdAt time.Time
	if err := s.db.GetContext(ctx, &createdAt, `SELECT created_at FROM workflows WHERE id=?`, workflowID); err == nil {
		wf.CreatedAt = createdAt
	}

	// Snapshot the restored state as the next version. The restore is already committed
	// so this is best-effort; a failure here is logged but does not roll back the restore.
	if vErr := s.CreateWorkflowVersion(ctx, wf); vErr != nil {
		slog.WarnContext(ctx, "workflow version store: post-restore snapshot failed",
			"workflow_id", workflowID, "restored_from", versionNum, "error", vErr)
	}

	return wf, nil
}
