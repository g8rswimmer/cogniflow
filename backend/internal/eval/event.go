package eval

import (
	"sync"
	"time"

	"github.com/g8rswimmer/cogniflow/internal/store"
)

// EvalEventType classifies one streaming event during an EvalRun.
type EvalEventType string

const (
	EvalEventTestCaseStarted   EvalEventType = "eval.test_case.started"
	EvalEventTestCaseCompleted EvalEventType = "eval.test_case.completed"
	EvalEventRunCompleted      EvalEventType = "eval.run.completed"
	EvalEventRunFailed         EvalEventType = "eval.run.failed"
)

// EvalRunSummary carries aggregate counts for an EvalRun terminal event.
type EvalRunSummary struct {
	TotalCases  int `json:"total_cases"`
	PassedCount int `json:"passed_count"`
	FailedCount int `json:"failed_count"`
	ErrorCount  int `json:"error_count"`
}

// EvalEvent is one status transition emitted during EvalRun execution.
// Events stream to WebSocket clients via EvalEventBus; they are not persisted.
type EvalEvent struct {
	EvalRunID    string                `json:"eval_run_id"`
	Type         EvalEventType         `json:"type"`
	Timestamp    time.Time             `json:"timestamp"`
	TestCaseName string                `json:"test_case_name,omitempty"`
	Result       *store.TestCaseResult `json:"result,omitempty"`
	Summary      *EvalRunSummary       `json:"summary,omitempty"`
}

// EvalEventBus fans out EvalEvents to all active subscribers for a given eval run.
// It mirrors the engine.EventBus pattern but carries EvalEvent instead of NodeEvent.
type EvalEventBus struct {
	mu          sync.RWMutex
	subscribers map[string][]chan EvalEvent
}

// NewEvalEventBus creates an empty EvalEventBus.
func NewEvalEventBus() *EvalEventBus {
	return &EvalEventBus{subscribers: make(map[string][]chan EvalEvent)}
}

// Subscribe registers a channel that will receive events for evalRunID.
// The returned cleanup function unregisters and closes the channel.
func (b *EvalEventBus) Subscribe(evalRunID string) (<-chan EvalEvent, func()) {
	ch := make(chan EvalEvent, 256)

	b.mu.Lock()
	b.subscribers[evalRunID] = append(b.subscribers[evalRunID], ch)
	b.mu.Unlock()

	cleanup := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		subs := b.subscribers[evalRunID]
		for i, s := range subs {
			if s == ch {
				b.subscribers[evalRunID] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
		if len(b.subscribers[evalRunID]) == 0 {
			delete(b.subscribers, evalRunID)
		}
		close(ch)
	}
	return ch, cleanup
}

// Publish sends an event to all subscribers for the event's EvalRunID.
// Slow subscribers are skipped (non-blocking send). Nil-safe.
func (b *EvalEventBus) Publish(event EvalEvent) {
	if b == nil {
		return
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subscribers[event.EvalRunID] {
		select {
		case ch <- event:
		default:
		}
	}
}
