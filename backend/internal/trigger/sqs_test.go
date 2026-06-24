package trigger

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

// mockSQSClient implements sqsAPI for testing.
type mockSQSClient struct {
	mu       sync.Mutex
	batches  [][]types.Message // ReceiveMessage returns successive batches
	batchIdx int
	recvErr  error
	deleted  []string // receipt handles of deleted messages
}

func (m *mockSQSClient) ReceiveMessage(ctx context.Context, _ *sqs.ReceiveMessageInput, _ ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error) {
	m.mu.Lock()
	if m.recvErr != nil {
		err := m.recvErr
		m.recvErr = nil // return once so the loop can make progress
		m.mu.Unlock()
		return nil, err
	}
	if m.batchIdx < len(m.batches) {
		batch := m.batches[m.batchIdx]
		m.batchIdx++
		m.mu.Unlock()
		return &sqs.ReceiveMessageOutput{Messages: batch}, nil
	}
	m.mu.Unlock()
	// Block until context is cancelled, mimicking SQS long-poll behaviour.
	<-ctx.Done()
	return nil, ctx.Err()
}

func (m *mockSQSClient) DeleteMessage(_ context.Context, params *sqs.DeleteMessageInput, _ ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleted = append(m.deleted, aws.ToString(params.ReceiptHandle))
	return &sqs.DeleteMessageOutput{}, nil
}

func newTestSQSConsumer(client sqsAPI, workflowID, queueURL string, disp Dispatcher) *sqsConsumer {
	return &sqsConsumer{
		client:     client,
		queueURL:   queueURL,
		workflowID: workflowID,
		dispatcher: disp,
	}
}

func TestValidateSQSConfig(t *testing.T) {
	tests := []struct {
		name     string
		queueURL string
		region   string
		wantErr  bool
	}{
		{"valid", "https://sqs.us-east-1.amazonaws.com/123/q", "us-east-1", false},
		{"empty url", "", "us-east-1", true},
		{"whitespace url", "  ", "us-east-1", true},
		{"empty region", "https://sqs.us-east-1.amazonaws.com/123/q", "", true},
		{"whitespace region", "https://sqs.us-east-1.amazonaws.com/123/q", "  ", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateSQSConfig(tc.queueURL, tc.region)
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateSQSConfig(%q,%q) error=%v, wantErr=%v", tc.queueURL, tc.region, err, tc.wantErr)
			}
		})
	}
}

func TestSQSConsumer_DispatchesMessages(t *testing.T) {
	payload := map[string]any{"order_id": "42"}
	body, _ := json.Marshal(payload)

	client := &mockSQSClient{
		batches: [][]types.Message{
			{{Body: aws.String(string(body)), ReceiptHandle: aws.String("rh-1")}},
		},
	}
	disp := &recordingDispatcher{}
	c := newTestSQSConsumer(client, "wf-sqs", "https://queue-url", disp)

	ctx, cancel := context.WithCancel(context.Background())
	go c.run(ctx)

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
	if req.WorkflowID != "wf-sqs" {
		t.Errorf("WorkflowID = %q, want %q", req.WorkflowID, "wf-sqs")
	}
	if req.TriggeredBy != "sqs" {
		t.Errorf("TriggeredBy = %q, want %q", req.TriggeredBy, "sqs")
	}
	if req.InitialData["order_id"] != "42" {
		t.Errorf("InitialData[order_id] = %v, want %q", req.InitialData["order_id"], "42")
	}
}

func TestSQSConsumer_DeletesMessageAfterDispatch(t *testing.T) {
	client := &mockSQSClient{
		batches: [][]types.Message{
			{{Body: aws.String(`{}`), ReceiptHandle: aws.String("rh-abc")}},
		},
	}
	disp := &recordingDispatcher{}
	c := newTestSQSConsumer(client, "wf-sqs", "https://queue-url", disp)

	ctx, cancel := context.WithCancel(context.Background())
	go c.run(ctx)

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		client.mu.Lock()
		n := len(client.deleted)
		client.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()

	client.mu.Lock()
	deleted := client.deleted
	client.mu.Unlock()
	if len(deleted) == 0 || deleted[0] != "rh-abc" {
		t.Errorf("expected message rh-abc to be deleted, got %v", deleted)
	}
}

func TestSQSConsumer_DoesNotDeleteOnDispatchError(t *testing.T) {
	client := &mockSQSClient{
		batches: [][]types.Message{
			{{Body: aws.String(`{}`), ReceiptHandle: aws.String("rh-fail")}},
		},
	}
	disp := &recordingDispatcher{err: errors.New("engine down")}
	c := newTestSQSConsumer(client, "wf-sqs", "https://queue-url", disp)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	go c.run(ctx)
	<-ctx.Done()

	client.mu.Lock()
	deleted := client.deleted
	client.mu.Unlock()
	if len(deleted) != 0 {
		t.Errorf("message should not be deleted on dispatch failure, got %v", deleted)
	}
}

func TestSQSConsumer_InvalidJSONUsesEmptyData(t *testing.T) {
	client := &mockSQSClient{
		batches: [][]types.Message{
			{{Body: aws.String("not-json"), ReceiptHandle: aws.String("rh-1")}},
		},
	}
	disp := &recordingDispatcher{}
	c := newTestSQSConsumer(client, "wf-sqs", "https://queue-url", disp)

	ctx, cancel := context.WithCancel(context.Background())
	go c.run(ctx)

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
		t.Fatal("expected dispatch even for invalid JSON")
	}
	if len(disp.calls[0].InitialData) != 0 {
		t.Errorf("expected empty InitialData for invalid JSON, got %v", disp.calls[0].InitialData)
	}
}

func TestSQSConsumer_BacksOffOnReceiveError(t *testing.T) {
	client := &mockSQSClient{
		recvErr: errors.New("network error"),
	}
	disp := &recordingDispatcher{}
	c := newTestSQSConsumer(client, "wf-sqs", "https://queue-url", disp)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	go c.run(ctx)
	<-ctx.Done()

	disp.mu.Lock()
	defer disp.mu.Unlock()
	if len(disp.calls) != 0 {
		t.Errorf("expected no dispatches on receive error, got %d", len(disp.calls))
	}
}
