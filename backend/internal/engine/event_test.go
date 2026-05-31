package engine

import (
	"testing"
	"time"
)

func TestEventBus_PublishSubscribe(t *testing.T) {
	bus := NewEventBus()

	ch, cleanup := bus.Subscribe("run-1")
	defer cleanup()

	event := NodeEvent{RunID: "run-1", NodeID: "n1", Type: EventNodeRunning, Timestamp: time.Now()}
	bus.Publish(event)

	select {
	case got := <-ch:
		if got.RunID != "run-1" || got.NodeID != "n1" {
			t.Errorf("unexpected event: %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestEventBus_NoSubscribersDoesNotPanic(t *testing.T) {
	bus := NewEventBus()
	// Should not panic or block.
	bus.Publish(NodeEvent{RunID: "run-x", NodeID: "n1", Type: EventNodeRunning, Timestamp: time.Now()})
}

func TestEventBus_MultipleSubscribers(t *testing.T) {
	bus := NewEventBus()

	ch1, cleanup1 := bus.Subscribe("run-1")
	defer cleanup1()
	ch2, cleanup2 := bus.Subscribe("run-1")
	defer cleanup2()

	bus.Publish(NodeEvent{RunID: "run-1", Type: EventRunSucceeded, Timestamp: time.Now()})

	recv := func(ch <-chan NodeEvent, label string) {
		select {
		case <-ch:
		case <-time.After(time.Second):
			t.Errorf("%s: timeout", label)
		}
	}
	recv(ch1, "ch1")
	recv(ch2, "ch2")
}

func TestEventBus_CleanupRemovesSubscriber(t *testing.T) {
	bus := NewEventBus()
	_, cleanup := bus.Subscribe("run-1")
	cleanup()

	bus.mu.RLock()
	count := len(bus.subscribers["run-1"])
	bus.mu.RUnlock()

	if count != 0 {
		t.Errorf("expected 0 subscribers after cleanup, got %d", count)
	}
}

func TestEventBus_IsolatesRuns(t *testing.T) {
	bus := NewEventBus()

	ch1, cleanup1 := bus.Subscribe("run-A")
	defer cleanup1()

	// Publish to run-B; ch1 (for run-A) should not receive it.
	bus.Publish(NodeEvent{RunID: "run-B", Type: EventNodeRunning, Timestamp: time.Now()})

	select {
	case ev := <-ch1:
		t.Errorf("run-A subscriber received run-B event: %+v", ev)
	case <-time.After(50 * time.Millisecond):
		// Correct: no cross-run leakage.
	}
}
