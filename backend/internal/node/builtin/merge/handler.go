// Package merge provides the merge built-in node for cogniflow.
package merge

import (
	"context"
	"encoding/json"

	"github.com/g8rswimmer/cogniflow/internal/node"
)

var inputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {}
}`)

var outputSchema = json.RawMessage(`{
  "type": "object",
  "description": "Flat map of all upstream node output fields merged together. Later branches overwrite earlier ones on key conflict."
}`)

// Handler implements the merge node type.
// The engine already handles fan-in synchronisation (waiting for all predecessors);
// this Execute is a simple passthrough that flattens the upstream outputs.
type Handler struct{}

// New returns a Handler for the "merge" node type.
func New() *Handler { return &Handler{} }

func (h *Handler) Meta() node.NodeMeta {
	return node.NodeMeta{
		TypeID:       "merge",
		DisplayName:  "Merge",
		Category:     "control",
		Description:  "Fan-in synchronisation point. Waits for all upstream branches and merges their outputs into a single flat map.",
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
	}
}

// Execute merges all upstream node outputs into a single flat map.
// The "_initial" key is excluded since it is already accessible to all nodes.
func (h *Handler) Execute(_ context.Context, input node.NodeInput) (node.NodeOutput, error) {
	out := make(map[string]any)
	for k, v := range input.UpstreamData {
		if k == "_initial" {
			continue
		}
		if m, ok := v.(map[string]any); ok {
			for mk, mv := range m {
				out[mk] = mv
			}
		}
	}
	return node.NodeOutput{Data: out}, nil
}
