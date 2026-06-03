// Package merge provides the merge built-in node for cogniflow.
package merge

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/g8rswimmer/cogniflow/internal/node"
)

var inputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {}
}`)

var outputSchema = json.RawMessage(`{
  "type": "object",
  "description": "Flat map of direct-predecessor node output fields merged together. On key conflict, the alphabetically-later node ID wins."
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

// Execute merges direct-predecessor node outputs into a single flat map.
// When DirectPredecessorIDs is populated by the engine, only those nodes
// are included — preventing transitive ancestors from silently contributing
// keys. Falls back to all UpstreamData entries (excluding "_initial") when
// DirectPredecessorIDs is absent (e.g. in unit tests).
// Keys are merged in sorted node-ID order so collision resolution is deterministic.
func (h *Handler) Execute(_ context.Context, input node.NodeInput) (node.NodeOutput, error) {
	ids := input.DirectPredecessorIDs
	if len(ids) == 0 {
		for k := range input.UpstreamData {
			if k != "_initial" {
				ids = append(ids, k)
			}
		}
	}
	sorted := make([]string, len(ids))
	copy(sorted, ids)
	sort.Strings(sorted)

	out := make(map[string]any)
	for _, id := range sorted {
		m, ok := input.UpstreamData[id].(map[string]any)
		if !ok {
			continue
		}
		for mk, mv := range m {
			out[mk] = mv
		}
	}
	return node.NodeOutput{Data: out}, nil
}
