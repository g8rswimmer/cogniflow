package mysql

import (
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/glebarez/go-sqlite"
)

// testSchema is a SQLite-compatible equivalent of the MySQL migrations.
// It omits MySQL-specific syntax (ENGINE, COLLATE, ON UPDATE, AUTO_INCREMENT)
// and uses SQLite's looser typing. Tested against the same SQL queries executed
// by WorkflowStore so any dialect mismatch surfaces here first.
const testSchema = `
CREATE TABLE IF NOT EXISTS workflows (
    id              TEXT     NOT NULL PRIMARY KEY,
    name            TEXT     NOT NULL,
    description     TEXT     NOT NULL DEFAULT '',
    trigger_kind    TEXT     NOT NULL DEFAULT 'manual',
    trigger_config  TEXT,
    timeout_seconds INTEGER  NOT NULL DEFAULT 300,
    created_at      DATETIME NOT NULL,
    updated_at      DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS workflow_nodes (
    id               TEXT    NOT NULL PRIMARY KEY,
    workflow_id      TEXT    NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
    type_id          TEXT    NOT NULL,
    label            TEXT    NOT NULL DEFAULT '',
    position_x       REAL    NOT NULL DEFAULT 0,
    position_y       REAL    NOT NULL DEFAULT 0,
    retry_max        INTEGER NOT NULL DEFAULT 0,
    retry_backoff_ms INTEGER NOT NULL DEFAULT 1000,
    output_parsers   TEXT
);

CREATE TABLE IF NOT EXISTS workflow_edges (
    id           TEXT NOT NULL PRIMARY KEY,
    workflow_id  TEXT NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
    source_id    TEXT NOT NULL,
    target_id    TEXT NOT NULL,
    branch_label TEXT
);

CREATE TABLE IF NOT EXISTS node_configs (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    node_id         TEXT    NOT NULL REFERENCES workflow_nodes(id) ON DELETE CASCADE,
    config_key      TEXT    NOT NULL,
    plain_value     TEXT,
    encrypted_value BLOB,
    is_sensitive    INTEGER NOT NULL DEFAULT 0,
    UNIQUE (node_id, config_key)
);

CREATE TABLE IF NOT EXISTS runs (
    id           TEXT     NOT NULL PRIMARY KEY,
    workflow_id  TEXT     NOT NULL,
    triggered_by TEXT     NOT NULL DEFAULT '',
    status       TEXT     NOT NULL DEFAULT 'running',
    started_at   DATETIME,
    finished_at  DATETIME,
    final_output TEXT,
    error_detail TEXT
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
    embedding   TEXT    NOT NULL DEFAULT '[]'
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

	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}
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
