package node

import (
	"context"
	"encoding/json"
)

// NodeInput carries the merged output context from all immediate upstream nodes
// plus this node's own persisted (decrypted) configuration values.
type NodeInput struct {
	// UpstreamData is keyed by node ID; each value is that node's output map.
	UpstreamData map[string]any
	// Config holds this node's configuration values (already decrypted).
	Config map[string]any
}

// NodeOutput is the data this node produces, forwarded to downstream nodes.
type NodeOutput struct {
	Data map[string]any
}

// NodeMeta is the static descriptor for a node type.
type NodeMeta struct {
	TypeID       string          `json:"type_id"`
	DisplayName  string          `json:"display_name"`
	Category     string          `json:"category"`
	Description  string          `json:"description"`
	InputSchema  json.RawMessage `json:"input_schema"`
	OutputSchema json.RawMessage `json:"output_schema"`
}

// NodeHandler is the interface every node type — built-in or plugin — must implement.
type NodeHandler interface {
	// Meta returns static metadata for this node type.
	Meta() NodeMeta

	// Execute runs the node's logic. A non-nil error marks the node as failed
	// and halts downstream execution.
	Execute(ctx context.Context, input NodeInput) (NodeOutput, error)
}
