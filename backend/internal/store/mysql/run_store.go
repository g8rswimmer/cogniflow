package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/g8rswimmer/cogniflow/internal/store"
)

// ---- Run CRUD ------------------------------------------------------------

func (s *WorkflowStore) CreateRun(ctx context.Context, r store.Run) (store.Run, error) {
	if r.ID == "" {
		r.ID = newUUID()
	}

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return store.Run{}, fmt.Errorf("run store: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Verify the workflow still exists inside this transaction. Without a FK
	// constraint on runs.workflow_id, this is the application-layer guard that
	// closes the TOCTOU window between engine.Run's GetWorkflow call and the
	// INSERT below.
	var exists int
	if err := tx.GetContext(ctx, &exists,
		`SELECT COUNT(*) FROM workflows WHERE id = ?`, r.WorkflowID,
	); err != nil {
		return store.Run{}, fmt.Errorf("run store: check workflow: %w", err)
	}
	if exists == 0 {
		return store.Run{}, fmt.Errorf("run store: workflow %q: %w", r.WorkflowID, store.ErrNotFound)
	}

	if _, err := tx.NamedExecContext(ctx,
		`INSERT INTO runs (id, workflow_id, triggered_by, status, started_at)
		 VALUES (:id, :workflow_id, :triggered_by, :status, :started_at)`,
		runWriteRow{
			ID:          r.ID,
			WorkflowID:  r.WorkflowID,
			TriggeredBy: string(r.TriggeredBy),
			Status:      string(r.Status),
			StartedAt:   r.StartedAt,
		},
	); err != nil {
		return store.Run{}, fmt.Errorf("run store: create run: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return store.Run{}, fmt.Errorf("run store: commit: %w", err)
	}
	return r, nil
}

func (s *WorkflowStore) UpdateRunStatus(ctx context.Context, runID string, status store.RunStatus, output map[string]any) error {
	now := time.Now().UTC()

	var finalOutput, errorDetail *string
	if output != nil {
		b, err := json.Marshal(output)
		if err != nil {
			return fmt.Errorf("run store: marshal output: %w", err)
		}
		s := string(b)
		if status == store.RunStatusSucceeded {
			finalOutput = &s
		} else {
			errorDetail = &s
		}
	}

	_, err := s.db.NamedExecContext(ctx,
		`UPDATE runs SET status=:status, finished_at=:finished_at,
		 final_output=:final_output, error_detail=:error_detail
		 WHERE id=:id`,
		runUpdateRow{
			ID:          runID,
			Status:      string(status),
			FinishedAt:  &now,
			FinalOutput: finalOutput,
			ErrorDetail: errorDetail,
		},
	)
	if err != nil {
		return fmt.Errorf("run store: update run status: %w", err)
	}
	return nil
}

func (s *WorkflowStore) GetRun(ctx context.Context, runID string) (store.Run, error) {
	var row dbRun
	err := s.db.GetContext(ctx, &row,
		`SELECT id, workflow_id, triggered_by, status, started_at, finished_at,
		        final_output, error_detail
		 FROM runs WHERE id=?`, runID)
	if errors.Is(err, sql.ErrNoRows) {
		return store.Run{}, store.ErrNotFound
	}
	if err != nil {
		return store.Run{}, fmt.Errorf("run store: get run: %w", err)
	}
	return rowToRun(row)
}

func (s *WorkflowStore) ListRuns(ctx context.Context, f store.RunFilter) ([]store.Run, error) {
	q := `SELECT id, workflow_id, triggered_by, status, started_at, finished_at,
	             final_output, error_detail
	      FROM runs WHERE workflow_id=?`
	args := []any{f.WorkflowID}

	if f.Status != "" {
		q += " AND status=?"
		args = append(args, string(f.Status))
	}
	if !f.Since.IsZero() {
		q += " AND started_at >= ?"
		args = append(args, f.Since)
	}
	if !f.Until.IsZero() {
		q += " AND started_at <= ?"
		args = append(args, f.Until)
	}
	q += " ORDER BY started_at DESC"
	if f.Limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", f.Limit)
	}

	var rows []dbRun
	if err := s.db.SelectContext(ctx, &rows, q, args...); err != nil {
		return nil, fmt.Errorf("run store: list runs: %w", err)
	}

	runs := make([]store.Run, 0, len(rows))
	for _, row := range rows {
		r, err := rowToRun(row)
		if err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	return runs, nil
}

// ---- row types -----------------------------------------------------------

type dbRun struct {
	ID          string     `db:"id"`
	WorkflowID  string     `db:"workflow_id"`
	TriggeredBy string     `db:"triggered_by"`
	Status      string     `db:"status"`
	StartedAt   *time.Time `db:"started_at"`
	FinishedAt  *time.Time `db:"finished_at"`
	FinalOutput []byte     `db:"final_output"`
	ErrorDetail []byte     `db:"error_detail"`
}

type runWriteRow struct {
	ID          string     `db:"id"`
	WorkflowID  string     `db:"workflow_id"`
	TriggeredBy string     `db:"triggered_by"`
	Status      string     `db:"status"`
	StartedAt   *time.Time `db:"started_at"`
}

type runUpdateRow struct {
	ID          string     `db:"id"`
	Status      string     `db:"status"`
	FinishedAt  *time.Time `db:"finished_at"`
	FinalOutput *string    `db:"final_output"`
	ErrorDetail *string    `db:"error_detail"`
}

func rowToRun(row dbRun) (store.Run, error) {
	r := store.Run{
		ID:          row.ID,
		WorkflowID:  row.WorkflowID,
		TriggeredBy: row.TriggeredBy,
		Status:      store.RunStatus(row.Status),
		StartedAt:   row.StartedAt,
		FinishedAt:  row.FinishedAt,
	}
	if len(row.FinalOutput) > 0 {
		if err := json.Unmarshal(row.FinalOutput, &r.FinalOutput); err != nil {
			return store.Run{}, fmt.Errorf("run store: unmarshal final_output: %w", err)
		}
	}
	if len(row.ErrorDetail) > 0 {
		if err := json.Unmarshal(row.ErrorDetail, &r.ErrorDetail); err != nil {
			return store.Run{}, fmt.Errorf("run store: unmarshal error_detail: %w", err)
		}
	}
	return r, nil
}
