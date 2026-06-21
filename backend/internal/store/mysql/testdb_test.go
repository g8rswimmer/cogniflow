package mysql

import (
	"context"
	"testing"
	"time"

	_ "github.com/glebarez/go-sqlite"
	"github.com/jmoiron/sqlx"
)

// testSchema is a SQLite-compatible equivalent of the MySQL migrations.
// It omits MySQL-specific syntax (ENGINE, COLLATE, ON UPDATE, AUTO_INCREMENT)
// and uses SQLite's looser typing. Tested against the same SQL queries executed
// by WorkflowStore so any dialect mismatch surfaces here first.
const testSchema = `
CREATE TABLE IF NOT EXISTS workflows (
    id                    TEXT     NOT NULL PRIMARY KEY,
    name                  TEXT     NOT NULL,
    description           TEXT     NOT NULL DEFAULT '',
    trigger_kind          TEXT     NOT NULL DEFAULT 'manual',
    trigger_config        TEXT,
    initial_data_schema   TEXT,
    timeout_seconds       INTEGER  NOT NULL DEFAULT 300,
    created_at            DATETIME NOT NULL,
    updated_at            DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS workflow_nodes (
    id               TEXT    NOT NULL,
    workflow_id      TEXT    NOT NULL,
    type_id          TEXT    NOT NULL,
    label            TEXT    NOT NULL DEFAULT '',
    position_x       REAL    NOT NULL DEFAULT 0,
    position_y       REAL    NOT NULL DEFAULT 0,
    retry_max        INTEGER NOT NULL DEFAULT 0,
    retry_backoff_ms INTEGER NOT NULL DEFAULT 1000,
    output_parsers   TEXT,
    PRIMARY KEY (workflow_id, id)
);

CREATE TABLE IF NOT EXISTS workflow_edges (
    id           TEXT    NOT NULL,
    workflow_id  TEXT    NOT NULL,
    source_id    TEXT    NOT NULL,
    target_id    TEXT    NOT NULL,
    branch_label TEXT,
    is_loop_back INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (workflow_id, id)
);

CREATE TABLE IF NOT EXISTS node_configs (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    workflow_id     TEXT    NOT NULL DEFAULT '',
    node_id         TEXT    NOT NULL,
    config_key      TEXT    NOT NULL,
    plain_value     TEXT,
    encrypted_value BLOB,
    is_sensitive    INTEGER NOT NULL DEFAULT 0,
    UNIQUE (workflow_id, node_id, config_key)
);

CREATE TABLE IF NOT EXISTS runs (
    id           TEXT     NOT NULL PRIMARY KEY,
    workflow_id  TEXT     NOT NULL,
    triggered_by TEXT     NOT NULL DEFAULT '',
    status       TEXT     NOT NULL DEFAULT 'running',
    started_at   DATETIME,
    finished_at  DATETIME,
    final_output TEXT,
    error_detail TEXT,
    node_results TEXT
);

CREATE TABLE IF NOT EXISTS rag_documents (
    id         TEXT     NOT NULL PRIMARY KEY,
    source     TEXT     NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS rag_chunks (
    id          TEXT    NOT NULL PRIMARY KEY,
    document_id TEXT    NOT NULL DEFAULT '',
    chunk_index INTEGER NOT NULL DEFAULT 0,
    chunk_text  TEXT    NOT NULL DEFAULT '',
    embedding   BLOB    NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS eval_suites (
    id               TEXT    NOT NULL PRIMARY KEY,
    workflow_id      TEXT    NOT NULL,
    name             TEXT    NOT NULL,
    description      TEXT    NOT NULL DEFAULT '',
    pass_threshold   REAL    NOT NULL DEFAULT 1.0,
    max_concurrency  INTEGER NOT NULL DEFAULT 1,
    workflow_deleted INTEGER NOT NULL DEFAULT 0,
    trigger_kind     TEXT    NOT NULL DEFAULT 'none',
    trigger_config   TEXT    NOT NULL DEFAULT '{}',
    created_at       DATETIME NOT NULL,
    updated_at       DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS eval_test_cases (
    id           TEXT     NOT NULL PRIMARY KEY,
    suite_id     TEXT     NOT NULL,
    name         TEXT     NOT NULL,
    description  TEXT     NOT NULL DEFAULT '',
    position     INTEGER  NOT NULL DEFAULT 0,
    initial_data TEXT     NOT NULL DEFAULT '{}',
    mocks        TEXT     NOT NULL DEFAULT '[]',
    graders      TEXT     NOT NULL DEFAULT '[]',
    created_at   DATETIME NOT NULL,
    updated_at   DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS eval_runs (
    id           TEXT     NOT NULL PRIMARY KEY,
    suite_id     TEXT     NOT NULL,
    triggered_by TEXT     NOT NULL DEFAULT 'manual',
    status       TEXT     NOT NULL DEFAULT 'pending',
    total_cases  INTEGER  NOT NULL DEFAULT 0,
    passed_count INTEGER  NOT NULL DEFAULT 0,
    failed_count INTEGER  NOT NULL DEFAULT 0,
    error_count  INTEGER  NOT NULL DEFAULT 0,
    started_at   DATETIME,
    finished_at  DATETIME,
    created_at   DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS eval_test_case_results (
    id                  TEXT     NOT NULL PRIMARY KEY,
    eval_run_id         TEXT     NOT NULL,
    test_case_id        TEXT     NOT NULL,
    test_case_name      TEXT     NOT NULL,
    workflow_run_id     TEXT     NOT NULL,
    workflow_run_status TEXT     NOT NULL,
    node_outputs        TEXT     NOT NULL DEFAULT '{}',
    grader_results      TEXT     NOT NULL DEFAULT '[]',
    passed              INTEGER  NOT NULL DEFAULT 0,
    created_at          DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS grader_registrations (
    type_id       TEXT     NOT NULL PRIMARY KEY,
    address       TEXT     NOT NULL,
    display_name  TEXT     NOT NULL,
    description   TEXT,
    config_schema TEXT     NOT NULL DEFAULT '{}',
    registered_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS workflow_versions (
    id             TEXT     NOT NULL PRIMARY KEY,
    workflow_id    TEXT     NOT NULL,
    version_number INTEGER  NOT NULL,
    node_count     INTEGER  NOT NULL DEFAULT 0,
    definition     TEXT     NOT NULL,
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`

// openTestDB opens an in-memory SQLite database, applies the test schema, and
// registers a cleanup to close the connection when the test ends.
// SetMaxOpenConns(1) ensures PRAGMA foreign_keys persists on the same connection.
func openTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(testSchema); err != nil {
		t.Fatalf("apply test schema: %v", err)
	}
	return db
}

// newTestStore returns a WorkflowStore backed by the in-memory SQLite DB.
func newTestStore(t *testing.T) *WorkflowStore {
	t.Helper()
	return NewWorkflowStore(openTestDB(t))
}

// insertTestWorkflow inserts a minimal workflow row so that CreateRun's
// workflow-existence check passes in tests that are focused on run behaviour
// rather than workflow creation.
func insertTestWorkflow(t *testing.T, s *WorkflowStore, id string) {
	t.Helper()
	_, err := s.db.ExecContext(context.Background(),
		`INSERT INTO workflows (id, name, trigger_kind, timeout_seconds, created_at, updated_at)
		 VALUES (?, ?, 'manual', 300, datetime('now'), datetime('now'))`,
		id, id,
	)
	if err != nil {
		t.Fatalf("insertTestWorkflow %q: %v", id, err)
	}
}

// withinSecond fails the test if want and got differ by more than one second.
// Used when comparing timestamps that survived a round-trip through SQLite.
func withinSecond(t *testing.T, label string, want, got time.Time) {
	t.Helper()
	d := want.Sub(got)
	if d < 0 {
		d = -d
	}
	if d > time.Second {
		t.Errorf("%s: want %v, got %v (diff %v)", label, want, got, d)
	}
}
