package grader_plugin

import (
	"errors"
	"fmt"
	"sync"
)

// ErrGraderAlreadyRegistered is returned by Register when a grader with the
// same type_id is already in the registry.
var ErrGraderAlreadyRegistered = errors.New("grader plugin registry: duplicate type_id")

// ErrGraderNotFound is returned by Unregister when the type_id is not found.
var ErrGraderNotFound = errors.New("grader plugin registry: type_id not found")

// GraderMeta holds the static descriptor for a registered grader plugin.
type GraderMeta struct {
	TypeID       string
	DisplayName  string
	Description  string
	ConfigSchema []byte
}

// GraderRegistry is a thread-safe registry of out-of-process grader plugins.
type GraderRegistry struct {
	mu      sync.RWMutex
	entries map[string]*grpcProxy
}

// NewGraderRegistry creates an empty GraderRegistry.
func NewGraderRegistry() *GraderRegistry {
	return &GraderRegistry{entries: make(map[string]*grpcProxy)}
}

// Register adds a grpcProxy under its type_id. Returns ErrGraderAlreadyRegistered on duplicate.
func (r *GraderRegistry) Register(proxy *grpcProxy) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.entries[proxy.meta.TypeID]; exists {
		return fmt.Errorf("%w %q", ErrGraderAlreadyRegistered, proxy.meta.TypeID)
	}
	r.entries[proxy.meta.TypeID] = proxy
	return nil
}

// Replace atomically substitutes the proxy for the given type_id. If the
// type_id is not yet registered, the proxy is inserted. The old connection is
// closed outside the lock to avoid blocking readers during cleanup.
func (r *GraderRegistry) Replace(proxy *grpcProxy) {
	r.mu.Lock()
	old := r.entries[proxy.meta.TypeID]
	r.entries[proxy.meta.TypeID] = proxy
	r.mu.Unlock()

	if old != nil {
		_ = old.Close()
	}
}

// Get returns the grpcProxy for a type_id and a boolean indicating whether it
// was found.
func (r *GraderRegistry) Get(typeID string) (*grpcProxy, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.entries[typeID]
	return p, ok
}

// Unregister removes the grpcProxy for typeID and closes its connection.
// Returns ErrGraderNotFound if not present.
func (r *GraderRegistry) Unregister(typeID string) error {
	r.mu.Lock()
	p, ok := r.entries[typeID]
	if !ok {
		r.mu.Unlock()
		return fmt.Errorf("grader type %q: %w", typeID, ErrGraderNotFound)
	}
	delete(r.entries, typeID)
	r.mu.Unlock()

	if p != nil {
		_ = p.Close()
	}
	return nil
}

// List returns the meta descriptors for all registered grader plugins.
func (r *GraderRegistry) List() []GraderMeta {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]GraderMeta, 0, len(r.entries))
	for _, p := range r.entries {
		out = append(out, p.meta)
	}
	return out
}

// Shutdown closes all registered grpcProxy connections.
func (r *GraderRegistry) Shutdown() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range r.entries {
		_ = p.Close()
	}
}
