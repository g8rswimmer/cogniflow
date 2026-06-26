// Package db_write provides the db.write built-in node for cogniflow.
package db_write

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/g8rswimmer/cogniflow/internal/node"
	"github.com/g8rswimmer/cogniflow/internal/node/builtin/nodeutil"
)

var inputSchema = json.RawMessage(`{
  "type": "object",
  "required": ["dsn", "query"],
  "properties": {
    "driver": { "type": "string", "title": "Driver", "default": "mysql", "description": "database/sql driver name (e.g. mysql, sqlite)" },
    "dsn":    { "type": "string", "title": "DSN",    "x-sensitive": true, "x-template": true },
    "query":  { "type": "string", "title": "SQL Statement", "description": "Parameterised INSERT, UPDATE, or DELETE. Use ? placeholders for dynamic values; pass the values in the params array." },
    "params": { "type": "array",  "title": "Parameters",    "items": { "type": "string", "x-template": true } }
  }
}`)

var outputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "rows_affected": { "type": "integer" }
  }
}`)

// Handler implements the db.write node type.
// It maintains a per-(driver,dsn) connection pool so that repeated executions
// of the same workflow reuse an existing *sql.DB rather than opening and closing
// a new one on every call.
type Handler struct {
	pool *nodeutil.DBPool
}

// New returns a Handler for the "db.write" node type.
func New() *Handler { return &Handler{pool: nodeutil.NewDBPool()} }

func (h *Handler) Meta() node.NodeMeta {
	return node.NodeMeta{
		TypeID:       "db.write",
		DisplayName:  "DB Write",
		Category:     "deterministic",
		Description:  "Execute a parameterised INSERT, UPDATE, or DELETE statement and return the rows affected.",
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
	}
}

// Execute runs the configured SQL write statement and returns the affected row count.
func (h *Handler) Execute(ctx context.Context, input node.NodeInput) (node.NodeOutput, error) {
	dsn, err := nodeutil.GetRequiredString(input.Config, "dsn")
	if err != nil {
		return node.NodeOutput{}, fmt.Errorf("db.write: %w", err)
	}

	rawQuery, err := nodeutil.GetRequiredString(input.Config, "query")
	if err != nil {
		return node.NodeOutput{}, fmt.Errorf("db.write: %w", err)
	}

	driver, _ := nodeutil.GetOptionalString(input.Config, "driver")
	if driver == "" {
		driver = "mysql"
	}

	renderedDSN, err := nodeutil.RenderTemplate(dsn, input.UpstreamData)
	if err != nil {
		return node.NodeOutput{}, fmt.Errorf("db.write: render dsn: %w", err)
	}

	args, err := nodeutil.ResolveParams(input.Config["params"], input.UpstreamData)
	if err != nil {
		return node.NodeOutput{}, fmt.Errorf("db.write: %w", err)
	}

	db, closeWhenDone, err := h.pool.Get(driver, renderedDSN)
	if err != nil {
		return node.NodeOutput{}, fmt.Errorf("db.write: open: %w", err)
	}
	if closeWhenDone {
		defer db.Close()
	}

	res, err := db.ExecContext(ctx, rawQuery, args...)
	if err != nil {
		return node.NodeOutput{}, fmt.Errorf("db.write: execute: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return node.NodeOutput{}, fmt.Errorf("db.write: rows affected: %w", err)
	}

	return node.NodeOutput{Data: map[string]any{
		"rows_affected": affected,
	}}, nil
}

