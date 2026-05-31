package node

import (
	"fmt"
	"sort"
	"sync"
)

// NodeRegistry is the central catalog of all available node types.
// It is safe for concurrent use.
type NodeRegistry struct {
	mu       sync.RWMutex
	handlers map[string]NodeHandler
}

// NewRegistry creates an empty NodeRegistry.
func NewRegistry() *NodeRegistry {
	return &NodeRegistry{
		handlers: make(map[string]NodeHandler),
	}
}

// Register adds a handler under its TypeID. Panics on duplicate TypeID.
func (r *NodeRegistry) Register(handler NodeHandler) {
	typeID := handler.Meta().TypeID
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.handlers[typeID]; exists {
		panic(fmt.Sprintf("node registry: duplicate type_id %q", typeID))
	}
	r.handlers[typeID] = handler
}

// Lookup returns the handler for a given TypeID, or an error if not found.
func (r *NodeRegistry) Lookup(typeID string) (NodeHandler, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	h, ok := r.handlers[typeID]
	if !ok {
		return nil, fmt.Errorf("node type %q not found", typeID)
	}
	return h, nil
}

// ListAll returns metadata for every registered node type, sorted by TypeID.
func (r *NodeRegistry) ListAll() []NodeMeta {
	r.mu.RLock()
	defer r.mu.RUnlock()
	metas := make([]NodeMeta, 0, len(r.handlers))
	for _, h := range r.handlers {
		metas = append(metas, h.Meta())
	}
	sort.Slice(metas, func(i, j int) bool {
		return metas[i].TypeID < metas[j].TypeID
	})
	return metas
}
