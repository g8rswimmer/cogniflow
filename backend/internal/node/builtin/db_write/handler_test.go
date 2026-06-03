package db_write

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	_ "github.com/glebarez/go-sqlite"

	"github.com/g8rswimmer/cogniflow/internal/node"
)

// setupSQLiteDB creates a temp SQLite file, applies DDL/DML, and returns the path.
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
	if meta.TypeID != "db.write" {
		t.Errorf("want db.write, got %s", meta.TypeID)
	}
	if meta.Category != "deterministic" {
		t.Errorf("want deterministic, got %s", meta.Category)
	}
}

func TestExecute_MissingDSN_ReturnsError(t *testing.T) {
	_, err := New().Execute(context.Background(), node.NodeInput{
		Config:       map[string]any{"query": "INSERT INTO t VALUES (1)"},
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
		openDB: func(driver, dsn string) (*sql.DB, error) {
			return nil, errors.New("connect refused")
		},
	}
	_, err := h.Execute(context.Background(), node.NodeInput{
		Config:       map[string]any{"dsn": "any", "query": "INSERT INTO t VALUES (1)"},
		UpstreamData: map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error from open failure")
	}
}

func TestExecute_Insert_ReturnsRowsAffected(t *testing.T) {
	path := setupSQLiteDB(t, "CREATE TABLE items (id INTEGER, name TEXT)")
	out, err := New().Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"driver": "sqlite",
			"dsn":    path,
			"query":  "INSERT INTO items VALUES (1, 'foo')",
		},
		UpstreamData: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Data["rows_affected"] != int64(1) {
		t.Errorf("want rows_affected 1, got %v", out.Data["rows_affected"])
	}
}

func TestExecute_Update_ReturnsRowsAffected(t *testing.T) {
	path := setupSQLiteDB(t,
		"CREATE TABLE items (id INTEGER, name TEXT)",
		"INSERT INTO items VALUES (1, 'old')",
		"INSERT INTO items VALUES (2, 'old')",
	)
	out, err := New().Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"driver": "sqlite",
			"dsn":    path,
			"query":  "UPDATE items SET name = 'new'",
		},
		UpstreamData: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Data["rows_affected"] != int64(2) {
		t.Errorf("want rows_affected 2, got %v", out.Data["rows_affected"])
	}
}

func TestExecute_Delete_ReturnsRowsAffected(t *testing.T) {
	path := setupSQLiteDB(t,
		"CREATE TABLE items (id INTEGER)",
		"INSERT INTO items VALUES (10)",
	)
	out, err := New().Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"driver": "sqlite",
			"dsn":    path,
			"query":  "DELETE FROM items WHERE id = 10",
		},
		UpstreamData: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Data["rows_affected"] != int64(1) {
		t.Errorf("want rows_affected 1, got %v", out.Data["rows_affected"])
	}
}

func TestExecute_WithParams(t *testing.T) {
	path := setupSQLiteDB(t,
		"CREATE TABLE items (id INTEGER, name TEXT)",
		"INSERT INTO items VALUES (1, 'original')",
	)
	out, err := New().Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"driver": "sqlite",
			"dsn":    path,
			"query":  "UPDATE items SET name = ? WHERE id = ?",
			"params": []any{"updated", "1"},
		},
		UpstreamData: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Data["rows_affected"] != int64(1) {
		t.Errorf("want rows_affected 1, got %v", out.Data["rows_affected"])
	}
}

func TestExecute_TemplateQuery(t *testing.T) {
	path := setupSQLiteDB(t, "CREATE TABLE events (name TEXT)")
	out, err := New().Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"driver": "sqlite",
			"dsn":    path,
			"query":  "INSERT INTO events VALUES ('{{._initial.event_name}}')",
		},
		UpstreamData: map[string]any{
			"_initial": map[string]any{"event_name": "signup"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Data["rows_affected"] != int64(1) {
		t.Errorf("want rows_affected 1, got %v", out.Data["rows_affected"])
	}
}

func TestExecute_ExecError_ReturnsError(t *testing.T) {
	path := setupSQLiteDB(t)
	_, err := New().Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"driver": "sqlite",
			"dsn":    path,
			"query":  "INSERT INTO nonexistent_table VALUES (1)",
		},
		UpstreamData: map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error for invalid table")
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
