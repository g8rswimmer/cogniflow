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

// ---- EvalSuite CRUD -------------------------------------------------------

func (s *WorkflowStore) CreateEvalSuite(ctx context.Context, suite store.EvalSuite) (store.EvalSuite, error) {
	if suite.ID == "" {
		suite.ID = newUUID()
	}
	if suite.PassThreshold == 0 {
		suite.PassThreshold = 1.0
	}
	if suite.MaxConcurrency == 0 {
		suite.MaxConcurrency = 1
	}
	if suite.TriggerKind == "" {
		suite.TriggerKind = "none"
	}
	now := time.Now().UTC()
	suite.CreatedAt = now
	suite.UpdatedAt = now

	trigCfg, err := marshalTriggerConfig(suite)
	if err != nil {
		return store.EvalSuite{}, fmt.Errorf("eval store: marshal trigger_config: %w", err)
	}

	orgID := store.OrgIDFrom(ctx)
	if orgID == "" {
		orgID = "00000000-0000-0000-0000-000000000001"
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO eval_suites
		 (id, org_id, workflow_id, name, description, pass_threshold, max_concurrency, workflow_deleted, trigger_kind, trigger_config, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		suite.ID, orgID, suite.WorkflowID, suite.Name, suite.Description,
		suite.PassThreshold, suite.MaxConcurrency, boolToInt(suite.WorkflowDeleted),
		suite.TriggerKind, trigCfg,
		suite.CreatedAt, suite.UpdatedAt,
	)
	if err != nil {
		return store.EvalSuite{}, fmt.Errorf("eval store: create suite: %w", err)
	}
	return suite, nil
}

func (s *WorkflowStore) GetEvalSuite(ctx context.Context, id string) (store.EvalSuite, error) {
	var row dbEvalSuite
	q := `SELECT id, workflow_id, name, description, pass_threshold, max_concurrency, workflow_deleted,
	             trigger_kind, trigger_config, created_at, updated_at
	      FROM eval_suites WHERE id=?`
	args := []any{id}
	if orgID := store.OrgIDFrom(ctx); orgID != "" {
		q += " AND org_id = ?"
		args = append(args, orgID)
	}
	err := s.db.GetContext(ctx, &row, q, args...)
	if errors.Is(err, sql.ErrNoRows) {
		return store.EvalSuite{}, store.ErrNotFound
	}
	if err != nil {
		return store.EvalSuite{}, fmt.Errorf("eval store: get suite: %w", err)
	}
	return rowToEvalSuite(row)
}

func (s *WorkflowStore) ListEvalSuites(ctx context.Context, workflowID string) ([]store.EvalSuiteSummary, error) {
	baseQuery := `SELECT es.id, es.workflow_id, es.name, es.description, es.pass_threshold,
		        es.max_concurrency, es.workflow_deleted, es.trigger_kind, es.trigger_config,
		        es.created_at, es.updated_at,
		        (SELECT COUNT(*) FROM eval_test_cases WHERE suite_id = es.id) AS test_case_count,
		        (SELECT status FROM eval_runs WHERE suite_id = es.id ORDER BY created_at DESC LIMIT 1) AS last_run_status,
		        (SELECT created_at FROM eval_runs WHERE suite_id = es.id ORDER BY created_at DESC LIMIT 1) AS last_run_at
		 FROM eval_suites es
		 WHERE es.workflow_id = ?`
	queryArgs := []any{workflowID}
	if orgID := store.OrgIDFrom(ctx); orgID != "" {
		baseQuery += ` AND es.org_id = ?`
		queryArgs = append(queryArgs, orgID)
	}
	baseQuery += ` ORDER BY es.created_at DESC`
	rows, err := s.db.QueryContext(ctx, baseQuery, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("eval store: list suites: %w", err)
	}
	defer rows.Close()

	var summaries []store.EvalSuiteSummary
	for rows.Next() {
		var row dbEvalSuiteSummary
		if err := rows.Scan(
			&row.ID, &row.WorkflowID, &row.Name, &row.Description,
			&row.PassThreshold, &row.MaxConcurrency, &row.WorkflowDeleted,
			&row.TriggerKind, &row.TriggerConfig,
			&row.CreatedAt, &row.UpdatedAt, &row.TestCaseCount,
			&row.LastRunStatus, &row.LastRunAt,
		); err != nil {
			return nil, fmt.Errorf("eval store: scan suite summary: %w", err)
		}
		s, err := rowToEvalSuite(row.dbEvalSuite)
		if err != nil {
			return nil, err
		}
		sum := store.EvalSuiteSummary{
			EvalSuite:     s,
			TestCaseCount: row.TestCaseCount,
			LastRunStatus: row.LastRunStatus,
		}
		if row.LastRunAt.Valid {
			t := row.LastRunAt.Time
			sum.LastRunAt = &t
		}
		summaries = append(summaries, sum)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("eval store: list suites rows: %w", err)
	}
	return summaries, nil
}

func (s *WorkflowStore) ListEvalSuitesByCronTrigger(ctx context.Context) ([]store.EvalSuite, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, workflow_id, name, description, pass_threshold, max_concurrency, workflow_deleted,
		        trigger_kind, trigger_config, created_at, updated_at
		 FROM eval_suites WHERE trigger_kind = 'cron' AND workflow_deleted = FALSE`)
	if err != nil {
		return nil, fmt.Errorf("eval store: list cron suites: %w", err)
	}
	defer rows.Close()

	var suites []store.EvalSuite
	for rows.Next() {
		var row dbEvalSuite
		if err := rows.Scan(
			&row.ID, &row.WorkflowID, &row.Name, &row.Description,
			&row.PassThreshold, &row.MaxConcurrency, &row.WorkflowDeleted,
			&row.TriggerKind, &row.TriggerConfig,
			&row.CreatedAt, &row.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("eval store: scan cron suite: %w", err)
		}
		suite, err := rowToEvalSuite(row)
		if err != nil {
			return nil, err
		}
		suites = append(suites, suite)
	}
	return suites, rows.Err()
}

func (s *WorkflowStore) UpdateEvalSuite(ctx context.Context, suite store.EvalSuite) (store.EvalSuite, error) {
	suite.UpdatedAt = time.Now().UTC()
	if suite.TriggerKind == "" {
		suite.TriggerKind = "none"
	}
	trigCfg, err := marshalTriggerConfig(suite)
	if err != nil {
		return store.EvalSuite{}, fmt.Errorf("eval store: marshal trigger_config: %w", err)
	}
	updateQ := `UPDATE eval_suites
	 SET name=?, description=?, pass_threshold=?, max_concurrency=?,
	     trigger_kind=?, trigger_config=?, updated_at=?
	 WHERE id=?`
	updateArgs := []any{suite.Name, suite.Description, suite.PassThreshold, suite.MaxConcurrency,
		suite.TriggerKind, trigCfg, suite.UpdatedAt, suite.ID}
	if orgID := store.OrgIDFrom(ctx); orgID != "" {
		updateQ += " AND org_id = ?"
		updateArgs = append(updateArgs, orgID)
	}
	result, err := s.db.ExecContext(ctx, updateQ, updateArgs...)
	if err != nil {
		return store.EvalSuite{}, fmt.Errorf("eval store: update suite: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return store.EvalSuite{}, store.ErrNotFound
	}
	return s.GetEvalSuite(ctx, suite.ID)
}

// DeleteEvalSuite cascades at the application layer:
// eval_test_case_results → eval_runs → eval_test_cases → eval_suites.
func (s *WorkflowStore) DeleteEvalSuite(ctx context.Context, id string) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("eval store: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM eval_test_case_results WHERE eval_run_id IN (SELECT id FROM eval_runs WHERE suite_id=?)`, id,
	); err != nil {
		return fmt.Errorf("eval store: delete test case results: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM eval_runs WHERE suite_id=?`, id); err != nil {
		return fmt.Errorf("eval store: delete eval runs: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM eval_test_cases WHERE suite_id=?`, id); err != nil {
		return fmt.Errorf("eval store: delete test cases: %w", err)
	}
	deleteQ := `DELETE FROM eval_suites WHERE id=?`
	deleteArgs := []any{id}
	if orgID := store.OrgIDFrom(ctx); orgID != "" {
		deleteQ += " AND org_id = ?"
		deleteArgs = append(deleteArgs, orgID)
	}
	result, err := tx.ExecContext(ctx, deleteQ, deleteArgs...)
	if err != nil {
		return fmt.Errorf("eval store: delete suite: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return store.ErrNotFound
	}

	return tx.Commit()
}

// ---- TestCase CRUD --------------------------------------------------------

func (s *WorkflowStore) CreateTestCase(ctx context.Context, tc store.TestCase) (store.TestCase, error) {
	if tc.ID == "" {
		tc.ID = newUUID()
	}
	now := time.Now().UTC()
	tc.CreatedAt = now
	tc.UpdatedAt = now

	initialDataJSON, err := marshalJSON(tc.InitialData)
	if err != nil {
		return store.TestCase{}, fmt.Errorf("eval store: marshal initial_data: %w", err)
	}
	mocksJSON, err := marshalJSON(tc.Mocks)
	if err != nil {
		return store.TestCase{}, fmt.Errorf("eval store: marshal mocks: %w", err)
	}
	gradersJSON, err := marshalJSON(tc.Graders)
	if err != nil {
		return store.TestCase{}, fmt.Errorf("eval store: marshal graders: %w", err)
	}

	// Compute position inside a transaction to prevent a TOCTOU race where two
	// concurrent inserts both read the same MAX(position) and produce duplicate positions.
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return store.TestCase{}, fmt.Errorf("eval store: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if tc.Position == 0 {
		var maxPos sql.NullInt64
		_ = tx.QueryRowContext(ctx,
			`SELECT MAX(position) FROM eval_test_cases WHERE suite_id=?`, tc.SuiteID,
		).Scan(&maxPos)
		if maxPos.Valid {
			tc.Position = int(maxPos.Int64) + 1
		}
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO eval_test_cases (id, suite_id, name, description, position, initial_data, mocks, graders, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		tc.ID, tc.SuiteID, tc.Name, tc.Description, tc.Position,
		initialDataJSON, mocksJSON, gradersJSON, tc.CreatedAt, tc.UpdatedAt,
	)
	if err != nil {
		return store.TestCase{}, fmt.Errorf("eval store: create test case: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return store.TestCase{}, fmt.Errorf("eval store: commit create test case: %w", err)
	}
	return tc, nil
}

func (s *WorkflowStore) GetTestCase(ctx context.Context, id string) (store.TestCase, error) {
	var row dbTestCase
	err := s.db.QueryRowContext(ctx,
		`SELECT id, suite_id, name, description, position, initial_data, mocks, graders, created_at, updated_at
		 FROM eval_test_cases WHERE id=?`, id,
	).Scan(&row.ID, &row.SuiteID, &row.Name, &row.Description, &row.Position,
		&row.InitialData, &row.Mocks, &row.Graders, &row.CreatedAt, &row.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return store.TestCase{}, store.ErrNotFound
	}
	if err != nil {
		return store.TestCase{}, fmt.Errorf("eval store: get test case: %w", err)
	}
	return rowToTestCase(row)
}

func (s *WorkflowStore) ListTestCases(ctx context.Context, suiteID string) ([]store.TestCase, error) {
	dbRows, err := s.db.QueryContext(ctx,
		`SELECT id, suite_id, name, description, position, initial_data, mocks, graders, created_at, updated_at
		 FROM eval_test_cases WHERE suite_id=? ORDER BY position ASC, created_at ASC`, suiteID)
	if err != nil {
		return nil, fmt.Errorf("eval store: list test cases: %w", err)
	}
	defer dbRows.Close()

	var cases []store.TestCase
	for dbRows.Next() {
		var row dbTestCase
		if err := dbRows.Scan(&row.ID, &row.SuiteID, &row.Name, &row.Description, &row.Position,
			&row.InitialData, &row.Mocks, &row.Graders, &row.CreatedAt, &row.UpdatedAt); err != nil {
			return nil, fmt.Errorf("eval store: scan test case: %w", err)
		}
		tc, err := rowToTestCase(row)
		if err != nil {
			return nil, err
		}
		cases = append(cases, tc)
	}
	return cases, dbRows.Err()
}

func (s *WorkflowStore) UpdateTestCase(ctx context.Context, tc store.TestCase) (store.TestCase, error) {
	tc.UpdatedAt = time.Now().UTC()

	initialDataJSON, err := marshalJSON(tc.InitialData)
	if err != nil {
		return store.TestCase{}, fmt.Errorf("eval store: marshal initial_data: %w", err)
	}
	mocksJSON, err := marshalJSON(tc.Mocks)
	if err != nil {
		return store.TestCase{}, fmt.Errorf("eval store: marshal mocks: %w", err)
	}
	gradersJSON, err := marshalJSON(tc.Graders)
	if err != nil {
		return store.TestCase{}, fmt.Errorf("eval store: marshal graders: %w", err)
	}

	result, err := s.db.ExecContext(ctx,
		`UPDATE eval_test_cases SET name=?, description=?, initial_data=?, mocks=?, graders=?, updated_at=?
		 WHERE id=?`,
		tc.Name, tc.Description, initialDataJSON, mocksJSON, gradersJSON, tc.UpdatedAt, tc.ID,
	)
	if err != nil {
		return store.TestCase{}, fmt.Errorf("eval store: update test case: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return store.TestCase{}, store.ErrNotFound
	}
	return s.GetTestCase(ctx, tc.ID)
}

func (s *WorkflowStore) DeleteTestCase(ctx context.Context, id string) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("eval store: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Cascade: delete results that reference this test case before deleting the case.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM eval_test_case_results WHERE test_case_id=?`, id,
	); err != nil {
		return fmt.Errorf("eval store: delete test case results: %w", err)
	}

	result, err := tx.ExecContext(ctx, `DELETE FROM eval_test_cases WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("eval store: delete test case: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return store.ErrNotFound
	}
	return tx.Commit()
}

func (s *WorkflowStore) ReorderTestCases(ctx context.Context, suiteID string, orderedIDs []string) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("eval store: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	for i, id := range orderedIDs {
		result, err := tx.ExecContext(ctx,
			`UPDATE eval_test_cases SET position=? WHERE id=? AND suite_id=?`, i, id, suiteID)
		if err != nil {
			return fmt.Errorf("eval store: reorder: %w", err)
		}
		n, _ := result.RowsAffected()
		if n == 0 {
			return fmt.Errorf("eval store: test case %q not in suite %q: %w", id, suiteID, store.ErrNotFound)
		}
	}
	return tx.Commit()
}

// ---- EvalRun CRUD ---------------------------------------------------------

func (s *WorkflowStore) CreateEvalRun(ctx context.Context, r store.EvalRun) (store.EvalRun, error) {
	if r.ID == "" {
		r.ID = newUUID()
	}
	if r.TriggeredBy == "" {
		r.TriggeredBy = "manual"
	}
	r.CreatedAt = time.Now().UTC()

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO eval_runs (id, suite_id, triggered_by, status, total_cases, passed_count, failed_count, error_count, workflow_version_number, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.SuiteID, r.TriggeredBy, string(r.Status), r.TotalCases,
		r.PassedCount, r.FailedCount, r.ErrorCount, r.WorkflowVersionNumber, r.CreatedAt,
	)
	if err != nil {
		return store.EvalRun{}, fmt.Errorf("eval store: create eval run: %w", err)
	}
	return r, nil
}

func (s *WorkflowStore) GetEvalRun(ctx context.Context, id string) (store.EvalRun, error) {
	var row dbEvalRun
	err := s.db.QueryRowContext(ctx,
		`SELECT id, suite_id, triggered_by, status, total_cases, passed_count, failed_count, error_count,
		        workflow_version_number, started_at, finished_at, created_at
		 FROM eval_runs WHERE id=?`, id,
	).Scan(&row.ID, &row.SuiteID, &row.TriggeredBy, &row.Status, &row.TotalCases,
		&row.PassedCount, &row.FailedCount, &row.ErrorCount,
		&row.WorkflowVersionNumber, &row.StartedAt, &row.FinishedAt, &row.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return store.EvalRun{}, store.ErrNotFound
	}
	if err != nil {
		return store.EvalRun{}, fmt.Errorf("eval store: get eval run: %w", err)
	}
	return rowToEvalRun(row), nil
}

func (s *WorkflowStore) ListEvalRuns(ctx context.Context, f store.EvalRunFilter) ([]store.EvalRun, error) {
	q := `SELECT id, suite_id, triggered_by, status, total_cases, passed_count, failed_count, error_count,
	             workflow_version_number, started_at, finished_at, created_at
	      FROM eval_runs WHERE suite_id=?`
	args := []any{f.SuiteID}

	if f.Status != "" {
		q += " AND status=?"
		args = append(args, string(f.Status))
	}
	q += " ORDER BY created_at DESC"
	if f.Limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", f.Limit)
	}
	if f.Offset > 0 {
		q += fmt.Sprintf(" OFFSET %d", f.Offset)
	}

	dbRows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("eval store: list eval runs: %w", err)
	}
	defer dbRows.Close()

	var runs []store.EvalRun
	for dbRows.Next() {
		var row dbEvalRun
		if err := dbRows.Scan(&row.ID, &row.SuiteID, &row.TriggeredBy, &row.Status,
			&row.TotalCases, &row.PassedCount, &row.FailedCount, &row.ErrorCount,
			&row.WorkflowVersionNumber, &row.StartedAt, &row.FinishedAt, &row.CreatedAt); err != nil {
			return nil, fmt.Errorf("eval store: scan eval run: %w", err)
		}
		runs = append(runs, rowToEvalRun(row))
	}
	return runs, dbRows.Err()
}

func (s *WorkflowStore) UpdateEvalRunStatus(ctx context.Context, runID string, status store.EvalRunStatus, counts store.EvalRunCounts) error {
	now := time.Now().UTC()
	var result sql.Result
	var err error

	switch status {
	case store.EvalRunRunning:
		// Only set status + started_at; never touch counts on the running transition
		// to avoid zeroing increments already committed by IncrementEvalRunCounts.
		result, err = s.db.ExecContext(ctx,
			`UPDATE eval_runs SET status=?, started_at=COALESCE(started_at, ?) WHERE id=?`,
			string(status), &now, runID,
		)
	default:
		// completed / failed: set final counts + finished_at together.
		result, err = s.db.ExecContext(ctx,
			`UPDATE eval_runs
			 SET status=?,
			     passed_count=?,
			     failed_count=?,
			     error_count=?,
			     finished_at=COALESCE(?, finished_at)
			 WHERE id=?`,
			string(status),
			counts.PassedCount, counts.FailedCount, counts.ErrorCount,
			&now, runID,
		)
	}
	if err != nil {
		return fmt.Errorf("eval store: update eval run status: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("eval store: eval run %q: %w", runID, store.ErrNotFound)
	}
	return nil
}

func (s *WorkflowStore) IncrementEvalRunCounts(ctx context.Context, runID string, delta store.EvalRunCounts) error {
	result, err := s.db.ExecContext(ctx,
		`UPDATE eval_runs
		 SET passed_count = passed_count + ?,
		     failed_count = failed_count + ?,
		     error_count  = error_count  + ?
		 WHERE id=?`,
		delta.PassedCount, delta.FailedCount, delta.ErrorCount, runID,
	)
	if err != nil {
		return fmt.Errorf("eval store: increment eval run counts: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("eval store: eval run %q: %w", runID, store.ErrNotFound)
	}
	return nil
}

// ---- TestCaseResult CRUD -------------------------------------------------

func (s *WorkflowStore) CreateTestCaseResult(ctx context.Context, r store.TestCaseResult) (store.TestCaseResult, error) {
	if r.ID == "" {
		r.ID = newUUID()
	}
	r.CreatedAt = time.Now().UTC()

	nodeOutputsJSON, err := marshalJSON(r.NodeOutputs)
	if err != nil {
		return store.TestCaseResult{}, fmt.Errorf("eval store: marshal node_outputs: %w", err)
	}
	graderResultsJSON, err := marshalJSON(r.GraderResults)
	if err != nil {
		return store.TestCaseResult{}, fmt.Errorf("eval store: marshal grader_results: %w", err)
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO eval_test_case_results
		 (id, eval_run_id, test_case_id, test_case_name, workflow_run_id, workflow_run_status, node_outputs, grader_results, passed, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.EvalRunID, r.TestCaseID, r.TestCaseName,
		r.WorkflowRunID, r.WorkflowRunStatus,
		nodeOutputsJSON, graderResultsJSON, boolToInt(r.Passed), r.CreatedAt,
	)
	if err != nil {
		return store.TestCaseResult{}, fmt.Errorf("eval store: create test case result: %w", err)
	}
	return r, nil
}

func (s *WorkflowStore) GetTestCaseResult(ctx context.Context, id string) (store.TestCaseResult, error) {
	var row dbTestCaseResult
	err := s.db.QueryRowContext(ctx,
		`SELECT id, eval_run_id, test_case_id, test_case_name, workflow_run_id, workflow_run_status,
		        node_outputs, grader_results, passed, created_at
		 FROM eval_test_case_results WHERE id=?`, id,
	).Scan(&row.ID, &row.EvalRunID, &row.TestCaseID, &row.TestCaseName,
		&row.WorkflowRunID, &row.WorkflowRunStatus,
		&row.NodeOutputs, &row.GraderResults, &row.Passed, &row.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return store.TestCaseResult{}, store.ErrNotFound
	}
	if err != nil {
		return store.TestCaseResult{}, fmt.Errorf("eval store: get test case result: %w", err)
	}
	return rowToTestCaseResult(row)
}

func (s *WorkflowStore) ListTestCaseResults(ctx context.Context, evalRunID string) ([]store.TestCaseResult, error) {
	dbRows, err := s.db.QueryContext(ctx,
		`SELECT id, eval_run_id, test_case_id, test_case_name, workflow_run_id, workflow_run_status,
		        node_outputs, grader_results, passed, created_at
		 FROM eval_test_case_results WHERE eval_run_id=? ORDER BY created_at ASC`, evalRunID)
	if err != nil {
		return nil, fmt.Errorf("eval store: list test case results: %w", err)
	}
	defer dbRows.Close()

	var results []store.TestCaseResult
	for dbRows.Next() {
		var row dbTestCaseResult
		if err := dbRows.Scan(&row.ID, &row.EvalRunID, &row.TestCaseID, &row.TestCaseName,
			&row.WorkflowRunID, &row.WorkflowRunStatus,
			&row.NodeOutputs, &row.GraderResults, &row.Passed, &row.CreatedAt); err != nil {
			return nil, fmt.Errorf("eval store: scan test case result: %w", err)
		}
		r, err := rowToTestCaseResult(row)
		if err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, dbRows.Err()
}

// ---- DB row types ---------------------------------------------------------

type dbEvalSuite struct {
	ID              string    `db:"id"`
	WorkflowID      string    `db:"workflow_id"`
	Name            string    `db:"name"`
	Description     string    `db:"description"`
	PassThreshold   float64   `db:"pass_threshold"`
	MaxConcurrency  int       `db:"max_concurrency"`
	WorkflowDeleted int       `db:"workflow_deleted"`
	TriggerKind     string    `db:"trigger_kind"`
	TriggerConfig   []byte    `db:"trigger_config"`
	CreatedAt       time.Time `db:"created_at"`
	UpdatedAt       time.Time `db:"updated_at"`
}

type dbEvalSuiteSummary struct {
	dbEvalSuite
	TestCaseCount int        `db:"test_case_count"`
	LastRunStatus *string    `db:"last_run_status"`
	LastRunAt     dbNullTime `db:"last_run_at"`
}

// dbNullTime is a nullable time scanner compatible with both MySQL (which returns
// time.Time when parseTime=true) and SQLite (which returns datetime strings).
type dbNullTime struct {
	Time  time.Time
	Valid bool
}

func (t *dbNullTime) Scan(value any) error {
	if value == nil {
		t.Valid = false
		return nil
	}
	switch v := value.(type) {
	case time.Time:
		t.Time, t.Valid = v, true
	case string:
		parsed := parseDateTime(v)
		if parsed.IsZero() {
			return fmt.Errorf("eval store: cannot parse time %q", v)
		}
		t.Time, t.Valid = parsed, true
	case []byte:
		return t.Scan(string(v))
	default:
		return fmt.Errorf("eval store: unsupported type %T for time scan", value)
	}
	return nil
}

type dbTestCase struct {
	ID          string    `db:"id"`
	SuiteID     string    `db:"suite_id"`
	Name        string    `db:"name"`
	Description string    `db:"description"`
	Position    int       `db:"position"`
	InitialData []byte    `db:"initial_data"`
	Mocks       []byte    `db:"mocks"`
	Graders     []byte    `db:"graders"`
	CreatedAt   time.Time `db:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"`
}

type dbEvalRun struct {
	ID                    string     `db:"id"`
	SuiteID               string     `db:"suite_id"`
	TriggeredBy           string     `db:"triggered_by"`
	Status                string     `db:"status"`
	TotalCases            int        `db:"total_cases"`
	PassedCount           int        `db:"passed_count"`
	FailedCount           int        `db:"failed_count"`
	ErrorCount            int        `db:"error_count"`
	WorkflowVersionNumber *int       `db:"workflow_version_number"`
	StartedAt             *time.Time `db:"started_at"`
	FinishedAt            *time.Time `db:"finished_at"`
	CreatedAt             time.Time  `db:"created_at"`
}

type dbTestCaseResult struct {
	ID                string    `db:"id"`
	EvalRunID         string    `db:"eval_run_id"`
	TestCaseID        string    `db:"test_case_id"`
	TestCaseName      string    `db:"test_case_name"`
	WorkflowRunID     string    `db:"workflow_run_id"`
	WorkflowRunStatus string    `db:"workflow_run_status"`
	NodeOutputs       []byte    `db:"node_outputs"`
	GraderResults     []byte    `db:"grader_results"`
	Passed            int       `db:"passed"`
	CreatedAt         time.Time `db:"created_at"`
}

// ---- row converters -------------------------------------------------------

func rowToEvalSuite(row dbEvalSuite) (store.EvalSuite, error) {
	s := store.EvalSuite{
		ID:              row.ID,
		WorkflowID:      row.WorkflowID,
		Name:            row.Name,
		Description:     row.Description,
		PassThreshold:   row.PassThreshold,
		MaxConcurrency:  row.MaxConcurrency,
		WorkflowDeleted: row.WorkflowDeleted != 0,
		TriggerKind:     row.TriggerKind,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}
	if len(row.TriggerConfig) > 0 {
		var cfg map[string]string
		if err := json.Unmarshal(row.TriggerConfig, &cfg); err != nil {
			return store.EvalSuite{}, fmt.Errorf("eval store: unmarshal trigger_config: %w", err)
		}
		s.CronExpr = cfg["cron_expr"]
		s.WebhookSecret = cfg["webhook_secret"]
	}
	return s, nil
}

func rowToTestCase(row dbTestCase) (store.TestCase, error) {
	tc := store.TestCase{
		ID:          row.ID,
		SuiteID:     row.SuiteID,
		Name:        row.Name,
		Description: row.Description,
		Position:    row.Position,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
	if len(row.InitialData) > 0 {
		if err := json.Unmarshal(row.InitialData, &tc.InitialData); err != nil {
			return store.TestCase{}, fmt.Errorf("eval store: unmarshal initial_data: %w", err)
		}
	}
	if tc.InitialData == nil {
		tc.InitialData = map[string]any{}
	}
	if len(row.Mocks) > 0 {
		if err := json.Unmarshal(row.Mocks, &tc.Mocks); err != nil {
			return store.TestCase{}, fmt.Errorf("eval store: unmarshal mocks: %w", err)
		}
	}
	if tc.Mocks == nil {
		tc.Mocks = []store.NodeMock{}
	}
	if len(row.Graders) > 0 {
		if err := json.Unmarshal(row.Graders, &tc.Graders); err != nil {
			return store.TestCase{}, fmt.Errorf("eval store: unmarshal graders: %w", err)
		}
	}
	if tc.Graders == nil {
		tc.Graders = []store.GraderDef{}
	}
	return tc, nil
}

func rowToEvalRun(row dbEvalRun) store.EvalRun {
	return store.EvalRun{
		ID:                    row.ID,
		SuiteID:               row.SuiteID,
		TriggeredBy:           row.TriggeredBy,
		Status:                store.EvalRunStatus(row.Status),
		TotalCases:            row.TotalCases,
		PassedCount:           row.PassedCount,
		FailedCount:           row.FailedCount,
		ErrorCount:            row.ErrorCount,
		WorkflowVersionNumber: row.WorkflowVersionNumber,
		StartedAt:             row.StartedAt,
		FinishedAt:            row.FinishedAt,
		CreatedAt:             row.CreatedAt,
	}
}

func rowToTestCaseResult(row dbTestCaseResult) (store.TestCaseResult, error) {
	r := store.TestCaseResult{
		ID:                row.ID,
		EvalRunID:         row.EvalRunID,
		TestCaseID:        row.TestCaseID,
		TestCaseName:      row.TestCaseName,
		WorkflowRunID:     row.WorkflowRunID,
		WorkflowRunStatus: row.WorkflowRunStatus,
		Passed:            row.Passed != 0,
		CreatedAt:         row.CreatedAt,
	}
	if len(row.NodeOutputs) > 0 {
		if err := json.Unmarshal(row.NodeOutputs, &r.NodeOutputs); err != nil {
			return store.TestCaseResult{}, fmt.Errorf("eval store: unmarshal node_outputs: %w", err)
		}
	}
	if r.NodeOutputs == nil {
		r.NodeOutputs = map[string]map[string]any{}
	}
	if len(row.GraderResults) > 0 {
		if err := json.Unmarshal(row.GraderResults, &r.GraderResults); err != nil {
			return store.TestCaseResult{}, fmt.Errorf("eval store: unmarshal grader_results: %w", err)
		}
	}
	if r.GraderResults == nil {
		r.GraderResults = []store.GraderResult{}
	}
	return r, nil
}

// ---- helpers ---------------------------------------------------------------

func marshalJSON(v any) (string, error) {
	if v == nil {
		return "{}", nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// marshalTriggerConfig serialises the trigger-specific fields of a suite to the
// trigger_config JSON blob. Only fields relevant to the trigger_kind are included.
func marshalTriggerConfig(s store.EvalSuite) (string, error) {
	cfg := map[string]string{}
	switch s.TriggerKind {
	case "cron":
		cfg["cron_expr"] = s.CronExpr
	case "webhook":
		cfg["webhook_secret"] = s.WebhookSecret
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// parseDateTime parses datetime strings as stored by SQLite in tests.
// MySQL uses time.Time natively so this is only needed for null datetime columns
// that come back as strings from the subquery in ListEvalSuites.
func parseDateTime(s string) time.Time {
	formats := []string{
		"2006-01-02T15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05.999",
		"2006-01-02 15:04:05",
		time.RFC3339,
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
