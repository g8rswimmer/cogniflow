// Package db_query provides the db.query built-in node for cogniflow.
package db_query

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/g8rswimmer/cogniflow/internal/node"
	"github.com/g8rswimmer/cogniflow/internal/node/builtin/nodeutil"
)

const defaultMaxRows = 1000

var inputSchema = json.RawMessage(`{
  "type": "object",
  "required": ["dsn", "query"],
  "properties": {
    "driver":   { "type": "string",  "title": "Driver",       "default": "mysql", "description": "database/sql driver name (e.g. mysql, sqlite)" },
    "dsn":      { "type": "string",  "title": "DSN",          "x-sensitive": true, "x-template": true },
    "query":    { "type": "string",  "title": "SQL Query",    "description": "Parameterised SQL SELECT. Use ? placeholders for dynamic values; pass the values in the params array." },
    "params":   { "type": "array",   "title": "Parameters",   "items": { "type": "string", "x-template": true } },
    "max_rows": { "type": "integer", "title": "Max Rows",     "default": 1000 }
  }
}`)

var outputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "rows":      { "type": "array", "items": { "type": "object" } },
    "row_count": { "type": "integer" }
  }
}`)

// Handler implements the db.query node type.
// It maintains a per-(driver,dsn) connection pool so that repeated executions
// of the same workflow reuse an existing *sql.DB rather than opening and closing
// a new one on every call.
type Handler struct {
	pool *nodeutil.DBPool
}

// New returns a Handler for the "db.query" node type.
func New() *Handler { return &Handler{pool: nodeutil.NewDBPool()} }

func (h *Handler) Meta() node.NodeMeta {
	return node.NodeMeta{
		TypeID:       "db.query",
		DisplayName:  "DB Query",
		Category:     "deterministic",
		Description:  "Execute a parameterised SELECT against a SQL database and return the result rows.",
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
	}
}

// Execute runs the configured SQL SELECT and returns the result rows.
func (h *Handler) Execute(ctx context.Context, input node.NodeInput) (node.NodeOutput, error) {
	dsn, err := nodeutil.GetRequiredString(input.Config, "dsn")
	if err != nil {
		return node.NodeOutput{}, fmt.Errorf("db.query: %w", err)
	}

	rawQuery, err := nodeutil.GetRequiredString(input.Config, "query")
	if err != nil {
		return node.NodeOutput{}, fmt.Errorf("db.query: %w", err)
	}

	driver, _ := nodeutil.GetOptionalString(input.Config, "driver")
	if driver == "" {
		driver = "mysql"
	}

	renderedDSN, err := nodeutil.RenderTemplate(dsn, input.UpstreamData)
	if err != nil {
		return node.NodeOutput{}, fmt.Errorf("db.query: render dsn: %w", err)
	}

	args, err := nodeutil.ResolveParams(input.Config["params"], input.UpstreamData)
	if err != nil {
		return node.NodeOutput{}, fmt.Errorf("db.query: %w", err)
	}

	maxRows := nodeutil.ToInt(input.Config["max_rows"], defaultMaxRows)
	if maxRows <= 0 {
		maxRows = defaultMaxRows
	}

	db, closeWhenDone, err := h.pool.Get(driver, renderedDSN)
	if err != nil {
		return node.NodeOutput{}, fmt.Errorf("db.query: open: %w", err)
	}
	if closeWhenDone {
		defer db.Close()
	}

	sqlRows, err := db.QueryContext(ctx, rawQuery, args...)
	if err != nil {
		return node.NodeOutput{}, fmt.Errorf("db.query: execute: %w", err)
	}
	defer sqlRows.Close()

	cols, err := sqlRows.Columns()
	if err != nil {
		return node.NodeOutput{}, fmt.Errorf("db.query: columns: %w", err)
	}

	var rows []map[string]any
	for sqlRows.Next() && len(rows) < maxRows {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := sqlRows.Scan(ptrs...); err != nil {
			return node.NodeOutput{}, fmt.Errorf("db.query: scan: %w", err)
		}
		row := make(map[string]any, len(cols))
		for i, col := range cols {
			// MySQL returns string/text columns as []byte; normalise to string.
			if b, ok := vals[i].([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = vals[i]
			}
		}
		rows = append(rows, row)
	}
	if err := sqlRows.Err(); err != nil {
		return node.NodeOutput{}, fmt.Errorf("db.query: rows: %w", err)
	}
	if rows == nil {
		rows = []map[string]any{}
	}

	return node.NodeOutput{Data: map[string]any{
		"rows":      rows,
		"row_count": len(rows),
	}}, nil
}

