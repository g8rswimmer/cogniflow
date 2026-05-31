package engine

import (
	"sync"
	"time"
)

// NodeEventType classifies a single execution event.
type NodeEventType string

const (
	EventNodePending   NodeEventType = "node.pending"
	EventNodeRunning   NodeEventType = "node.running"
	EventNodeSucceeded NodeEventType = "node.succeeded"
	EventNodeFailed    NodeEventType = "node.failed"
	EventRunSucceeded  NodeEventType = "run.succeeded"
	EventRunFailed     NodeEventType = "run.failed"
)

// NodeEvent is one status transition emitted during workflow execution.
// Events stream to WebSocket clients via EventBus; they are not stored in the DB.
type NodeEvent struct {
	RunID     string         `json:"run_id"`
	NodeID    string         `json:"node_id"`
	Type      NodeEventType  `json:"type"`
	Timestamp time.Time      `json:"timestamp"`
	Output    map[string]any `json:"output,omitempty"` // set on node.succeeded
	Error     string         `json:"error,omitempty"`  // set on node.failed / run.failed
}

// EventBus fans out NodeEvents to all active subscribers for a given run.
// In M4, WebSocket handlers subscribe here; in M3 the bus is wired but unused.
type EventBus struct {
	mu          sync.RWMutex
	subscribers map[string][]chan NodeEvent // run_id → subscriber channels
}

// NewEventBus creates an empty EventBus.
func NewEventBus() *EventBus {
	return &EventBus{subscribers: make(map[string][]chan NodeEvent)}
}

// Subscribe registers a channel that will receive events for runID.
// The returned cleanup function unregisters and closes the channel.
func (b *EventBus) Subscribe(runID string) (<-chan NodeEvent, func()) {
	ch := make(chan NodeEvent, 64)

	b.mu.Lock()
	b.subscribers[runID] = append(b.subscribers[runID], ch)
	b.mu.Unlock()

	cleanup := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		subs := b.subscribers[runID]
		for i, s := range subs {
			if s == ch {
				b.subscribers[runID] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
		if len(b.subscribers[runID]) == 0 {
			delete(b.subscribers, runID)
		}
		close(ch)
	}
	return ch, cleanup
}

// Publish sends an event to all subscribers for the event's RunID.
// Slow subscribers are skipped (non-blocking send).
func (b *EventBus) Publish(event NodeEvent) {
	b.mu.RLock()
	subs := b.subscribers[event.RunID]
	b.mu.RUnlock()

	for _, ch := range subs {
		select {
		case ch <- event:
		default:
		}
	}
}
