// Package data_transform provides the data.transform built-in node for cogniflow.
package data_transform

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/g8rswimmer/cogniflow/internal/node"
	"github.com/g8rswimmer/cogniflow/internal/node/builtin/nodeutil"
)

var inputSchema = json.RawMessage(`{
  "type": "object",
  "required": ["fields"],
  "properties": {
    "fields": {
      "type": "object",
      "title": "Output Fields",
      "description": "Map of output key to Go template string. Each value is rendered with upstream data as context.",
      "additionalProperties": { "type": "string", "x-template": true }
    }
  }
}`)

var outputSchema = json.RawMessage(`{
  "type": "object",
  "description": "Keys are the output field names defined in the fields config; values are the rendered template strings."
}`)

// Handler implements the data.transform node type.
type Handler struct{}

// New returns a Handler for the "data.transform" node type.
func New() *Handler { return &Handler{} }

func (h *Handler) Meta() node.NodeMeta {
	return node.NodeMeta{
		TypeID:       "data.transform",
		DisplayName:  "Data Transform",
		Category:     "deterministic",
		Description:  "Transforms upstream data into a new shape by rendering Go template expressions for each output field.",
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
	}
}

// Execute renders each field template against the upstream data and returns the results.
func (h *Handler) Execute(_ context.Context, input node.NodeInput) (node.NodeOutput, error) {
	fieldsRaw, ok := input.Config["fields"]
	if !ok {
		return node.NodeOutput{Data: map[string]any{}}, nil
	}

	fields, ok := fieldsRaw.(map[string]any)
	if !ok {
		return node.NodeOutput{}, fmt.Errorf("data.transform: fields must be an object")
	}

	out := make(map[string]any, len(fields))
	for key, val := range fields {
		tmpl, ok := val.(string)
		if !ok {
			out[key] = val
			continue
		}
		rendered, err := nodeutil.RenderTemplate(tmpl, input.UpstreamData)
		if err != nil {
			return node.NodeOutput{}, fmt.Errorf("data.transform: field %q: %w", key, err)
		}
		out[key] = rendered
	}

	return node.NodeOutput{Data: out}, nil
}
