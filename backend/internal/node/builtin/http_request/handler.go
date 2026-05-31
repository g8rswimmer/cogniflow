package httprequest

import (
	"context"
	"encoding/json"

	"github.com/g8rswimmer/cogniflow/internal/node"
)

var inputSchema = json.RawMessage(`{
  "type": "object",
  "required": ["url", "method"],
  "properties": {
    "url":     { "type": "string",  "title": "URL" },
    "method":  { "type": "string",  "title": "Method", "enum": ["GET","POST","PUT","PATCH","DELETE"], "default": "GET" },
    "headers": { "type": "object",  "title": "Headers", "additionalProperties": { "type": "string" } },
    "body":    { "type": "string",  "title": "Body" },
    "timeout_seconds": { "type": "integer", "title": "Timeout (seconds)", "default": 30 }
  }
}`)

var outputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "status_code": { "type": "integer" },
    "body":        { "type": "string" },
    "headers":     { "type": "object", "additionalProperties": { "type": "string" } }
  }
}`)

// Handler implements the http.request built-in node.
type Handler struct{}

// New returns a new HTTPRequest node handler.
func New() *Handler { return &Handler{} }

func (h *Handler) Meta() node.NodeMeta {
	return node.NodeMeta{
		TypeID:       "http.request",
		DisplayName:  "HTTP Request",
		Category:     "deterministic",
		Description:  "Make an HTTP request and return the status code, headers, and body.",
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
	}
}

// Execute is stubbed for M2; full implementation arrives in M3.
func (h *Handler) Execute(_ context.Context, _ node.NodeInput) (node.NodeOutput, error) {
	return node.NodeOutput{Data: map[string]any{}}, nil
}
