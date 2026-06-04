package node

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"sync"
)

// ErrNodeNotFound is returned by Lookup when a type ID is not registered.
var ErrNodeNotFound = errors.New("node: type not found")

// ErrDuplicateTypeID is returned by TryRegister when a handler with the same
// TypeID is already registered.
var ErrDuplicateTypeID = errors.New("node registry: duplicate type_id")

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
		return nil, fmt.Errorf("node type %q: %w", typeID, ErrNodeNotFound)
	}
	return h, nil
}

// TryRegister adds a handler under its TypeID, returning an error if the TypeID
// is already registered. Unlike Register, it does not panic on collision.
func (r *NodeRegistry) TryRegister(handler NodeHandler) error {
	typeID := handler.Meta().TypeID
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.handlers[typeID]; exists {
		return fmt.Errorf("%w %q", ErrDuplicateTypeID, typeID)
	}
	r.handlers[typeID] = handler
	return nil
}

// Unregister removes a handler from the registry by TypeID, calling Close()
// if the handler implements io.Closer. Returns ErrNodeNotFound if absent.
func (r *NodeRegistry) Unregister(typeID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	h, ok := r.handlers[typeID]
	if !ok {
		return fmt.Errorf("node type %q: %w", typeID, ErrNodeNotFound)
	}
	delete(r.handlers, typeID)
	if c, ok := h.(io.Closer); ok {
		_ = c.Close()
	}
	return nil
}

// Replace atomically substitutes the handler for the given TypeID, or registers
// it if absent. The old handler's Close() is called outside the write lock to
// avoid holding the lock during a potentially slow gRPC drain. Use this instead
// of Unregister+TryRegister to avoid leaving a gap in the registry.
func (r *NodeRegistry) Replace(handler NodeHandler) {
	typeID := handler.Meta().TypeID
	r.mu.Lock()
	old := r.handlers[typeID]
	r.handlers[typeID] = handler
	r.mu.Unlock()
	if old != nil {
		if c, ok := old.(io.Closer); ok {
			_ = c.Close()
		}
	}
}

// Shutdown closes any registered handler that implements io.Closer (e.g. gRPC
// plugin connections). Built-in handlers that do not implement io.Closer are
// silently skipped. Call once during server shutdown.
func (r *NodeRegistry) Shutdown() {
	r.mu.RLock()
	closers := make([]io.Closer, 0, len(r.handlers))
	for _, h := range r.handlers {
		if c, ok := h.(io.Closer); ok {
			closers = append(closers, c)
		}
	}
	r.mu.RUnlock()
	for _, c := range closers {
		_ = c.Close()
	}
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
