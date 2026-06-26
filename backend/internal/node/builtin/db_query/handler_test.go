package db_query

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	_ "github.com/glebarez/go-sqlite"

	"github.com/g8rswimmer/cogniflow/internal/node"
	"github.com/g8rswimmer/cogniflow/internal/node/builtin/nodeutil"
)

// setupSQLiteDB creates a temporary SQLite file, runs the provided DDL+DML,
// and returns the file path to use as the DSN.
func setupSQLiteDB(t *testing.T, stmts ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	return path
}

func TestHandler_Meta(t *testing.T) {
	h := New()
	meta := h.Meta()
	if meta.TypeID != "db.query" {
		t.Errorf("want db.query, got %s", meta.TypeID)
	}
	if meta.Category != "deterministic" {
		t.Errorf("want deterministic, got %s", meta.Category)
	}
}

func TestExecute_MissingDSN_ReturnsError(t *testing.T) {
	_, err := New().Execute(context.Background(), node.NodeInput{
		Config:       map[string]any{"query": "SELECT 1"},
		UpstreamData: map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error for missing dsn")
	}
}

func TestExecute_MissingQuery_ReturnsError(t *testing.T) {
	_, err := New().Execute(context.Background(), node.NodeInput{
		Config:       map[string]any{"dsn": ":memory:"},
		UpstreamData: map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error for missing query")
	}
}

func TestExecute_OpenError_ReturnsError(t *testing.T) {
	h := &Handler{
		pool: nodeutil.NewTestPool(func(driver, dsn string) (*sql.DB, error) {
			return nil, errors.New("connect refused")
		}),
	}
	_, err := h.Execute(context.Background(), node.NodeInput{
		Config:       map[string]any{"dsn": "any", "query": "SELECT 1"},
		UpstreamData: map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error from open failure")
	}
}

func TestExecute_QueryError_ReturnsError(t *testing.T) {
	path := setupSQLiteDB(t)
	_, err := New().Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"driver": "sqlite",
			"dsn":    path,
			"query":  "SELECT * FROM nonexistent_table",
		},
		UpstreamData: map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error for invalid query")
	}
}

func TestExecute_Success_ReturnsRows(t *testing.T) {
	path := setupSQLiteDB(t,
		"CREATE TABLE items (id INTEGER, name TEXT)",
		"INSERT INTO items VALUES (1, 'apple')",
		"INSERT INTO items VALUES (2, 'banana')",
	)
	out, err := New().Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"driver": "sqlite",
			"dsn":    path,
			"query":  "SELECT id, name FROM items ORDER BY id",
		},
		UpstreamData: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rows, ok := out.Data["rows"].([]map[string]any)
	if !ok {
		t.Fatalf("rows not []map[string]any, got %T", out.Data["rows"])
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(rows))
	}
	if out.Data["row_count"] != 2 {
		t.Errorf("want row_count 2, got %v", out.Data["row_count"])
	}
}

func TestExecute_EmptyResult(t *testing.T) {
	path := setupSQLiteDB(t,
		"CREATE TABLE items (id INTEGER, name TEXT)",
	)
	out, err := New().Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"driver": "sqlite",
			"dsn":    path,
			"query":  "SELECT id, name FROM items",
		},
		UpstreamData: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rows := out.Data["rows"].([]map[string]any)
	if len(rows) != 0 {
		t.Errorf("want 0 rows, got %d", len(rows))
	}
}

func TestExecute_MaxRowsLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := db.Exec("CREATE TABLE nums (n INTEGER)"); err != nil {
		t.Fatalf("create: %v", err)
	}
	for i := 0; i < 10; i++ {
		if _, err := db.Exec(fmt.Sprintf("INSERT INTO nums VALUES (%d)", i)); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	db.Close()

	out, err := New().Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"driver":   "sqlite",
			"dsn":      path,
			"query":    "SELECT n FROM nums",
			"max_rows": 3,
		},
		UpstreamData: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rows := out.Data["rows"].([]map[string]any)
	if len(rows) != 3 {
		t.Errorf("want 3 rows (max_rows), got %d", len(rows))
	}
}

func TestExecute_WithParams(t *testing.T) {
	path := setupSQLiteDB(t,
		"CREATE TABLE items (id INTEGER, name TEXT)",
		"INSERT INTO items VALUES (1, 'alpha')",
		"INSERT INTO items VALUES (2, 'beta')",
	)
	out, err := New().Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"driver": "sqlite",
			"dsn":    path,
			"query":  "SELECT name FROM items WHERE id = ?",
			"params": []any{"1"},
		},
		UpstreamData: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rows := out.Data["rows"].([]map[string]any)
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
}

// TestExecute_ParamQuery verifies that a dynamic value is safely passed via the
// params array (parameterised binding) rather than inlined into the query string.
func TestExecute_ParamQuery(t *testing.T) {
	path := setupSQLiteDB(t,
		"CREATE TABLE items (id INTEGER, name TEXT)",
		"INSERT INTO items VALUES (5, 'gamma')",
	)
	out, err := New().Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"driver": "sqlite",
			"dsn":    path,
			"query":  "SELECT name FROM items WHERE id = ?",
			"params": []any{"5"},
		},
		UpstreamData: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rows := out.Data["rows"].([]map[string]any)
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d: %v", len(rows), rows)
	}
}

func TestExecute_InvalidParams_ReturnsError(t *testing.T) {
	path := setupSQLiteDB(t)
	_, err := New().Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"driver": "sqlite",
			"dsn":    path,
			"query":  "SELECT 1",
			"params": "not-an-array",
		},
		UpstreamData: map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error for non-array params")
	}
}
