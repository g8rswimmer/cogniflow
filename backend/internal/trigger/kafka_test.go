package trigger

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
)

// recordingDispatcher records all Dispatch calls made to it.
type recordingDispatcher struct {
	mu    sync.Mutex
	calls []RunRequest
	err   error
}

func (d *recordingDispatcher) Dispatch(_ context.Context, req RunRequest) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls = append(d.calls, req)
	return "run-id", d.err
}

// mockKafkaReader feeds a fixed sequence of messages then blocks until ctx is cancelled.
type mockKafkaReader struct {
	messages []kafka.Message
	idx      int
	err      error // if set, returned after all messages are consumed
	closed   bool
}

func (m *mockKafkaReader) ReadMessage(ctx context.Context) (kafka.Message, error) {
	if m.idx < len(m.messages) {
		msg := m.messages[m.idx]
		m.idx++
		return msg, nil
	}
	if m.err != nil {
		err := m.err
		m.err = nil // return once so backoff loop can make progress
		return kafka.Message{}, err
	}
	// Block until context is cancelled.
	<-ctx.Done()
	return kafka.Message{}, ctx.Err()
}

func (m *mockKafkaReader) Close() error {
	m.closed = true
	return nil
}

func newTestKafkaConsumer(reader kafkaReader, workflowID string, disp Dispatcher) *kafkaConsumer {
	return &kafkaConsumer{
		reader:     reader,
		workflowID: workflowID,
		dispatcher: disp,
	}
}

func TestValidateKafkaConfig(t *testing.T) {
	tests := []struct {
		name    string
		brokers string
		topic   string
		wantErr bool
	}{
		{"valid", "localhost:9092", "my-topic", false},
		{"empty brokers", "", "my-topic", true},
		{"whitespace brokers", "  ", "my-topic", true},
		{"empty topic", "localhost:9092", "", true},
		{"whitespace topic", "localhost:9092", "  ", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateKafkaConfig(tc.brokers, tc.topic)
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateKafkaConfig(%q,%q) error=%v, wantErr=%v", tc.brokers, tc.topic, err, tc.wantErr)
			}
		})
	}
}

func TestNewKafkaConsumer_ValidationError(t *testing.T) {
	_, err := newKafkaConsumer("wf-1", "", "topic", "", &recordingDispatcher{})
	if err == nil {
		t.Error("expected error for empty brokers")
	}
	_, err = newKafkaConsumer("wf-1", "localhost:9092", "", "", &recordingDispatcher{})
	if err == nil {
		t.Error("expected error for empty topic")
	}
}

func TestKafkaConsumer_DispatchesMessages(t *testing.T) {
	payload := map[string]any{"key": "value"}
	body, _ := json.Marshal(payload)

	reader := &mockKafkaReader{
		messages: []kafka.Message{
			{Value: body},
		},
	}
	disp := &recordingDispatcher{}
	c := newTestKafkaConsumer(reader, "wf-1", disp)

	ctx, cancel := context.WithCancel(context.Background())
	c.start(ctx)

	// Wait for the message to be dispatched.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		disp.mu.Lock()
		n := len(disp.calls)
		disp.mu.Unlock()
		if n >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()

	disp.mu.Lock()
	defer disp.mu.Unlock()
	if len(disp.calls) == 0 {
		t.Fatal("expected at least one dispatch call")
	}
	req := disp.calls[0]
	if req.WorkflowID != "wf-1" {
		t.Errorf("WorkflowID = %q, want %q", req.WorkflowID, "wf-1")
	}
	if req.TriggeredBy != "kafka" {
		t.Errorf("TriggeredBy = %q, want %q", req.TriggeredBy, "kafka")
	}
	if req.InitialData["key"] != "value" {
		t.Errorf("InitialData[key] = %v, want %q", req.InitialData["key"], "value")
	}
}

func TestKafkaConsumer_InvalidJSONUsesEmptyData(t *testing.T) {
	reader := &mockKafkaReader{
		messages: []kafka.Message{
			{Value: []byte("not-json")},
		},
	}
	disp := &recordingDispatcher{}
	c := newTestKafkaConsumer(reader, "wf-1", disp)

	ctx, cancel := context.WithCancel(context.Background())
	c.start(ctx)

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		disp.mu.Lock()
		n := len(disp.calls)
		disp.mu.Unlock()
		if n >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()

	disp.mu.Lock()
	defer disp.mu.Unlock()
	if len(disp.calls) == 0 {
		t.Fatal("expected a dispatch call even for invalid JSON")
	}
	if len(disp.calls[0].InitialData) != 0 {
		t.Errorf("expected empty InitialData for invalid JSON, got %v", disp.calls[0].InitialData)
	}
}

func TestKafkaConsumer_ReaderClosedOnCancel(t *testing.T) {
	reader := &mockKafkaReader{}
	c := newTestKafkaConsumer(reader, "wf-1", &recordingDispatcher{})

	ctx, cancel := context.WithCancel(context.Background())
	c.start(ctx)
	cancel()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if reader.closed {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !reader.closed {
		t.Error("expected reader.Close() to be called after context cancellation")
	}
}

func TestKafkaConsumer_BacksOffOnReadError(t *testing.T) {
	reader := &mockKafkaReader{
		err: errors.New("connection refused"),
	}
	disp := &recordingDispatcher{}
	c := newTestKafkaConsumer(reader, "wf-1", disp)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	c.start(ctx)
	<-ctx.Done()
	// No dispatch should have occurred (no messages).
	disp.mu.Lock()
	defer disp.mu.Unlock()
	if len(disp.calls) != 0 {
		t.Errorf("expected no dispatches on read error, got %d", len(disp.calls))
	}
}
