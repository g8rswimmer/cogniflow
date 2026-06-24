package trigger

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
)

// ValidateKafkaConfig checks that the Kafka-specific trigger fields are present.
func ValidateKafkaConfig(brokers, topic string) error {
	if strings.TrimSpace(brokers) == "" {
		return fmt.Errorf("kafka_brokers is required when trigger.kind is \"kafka\"")
	}
	if strings.TrimSpace(topic) == "" {
		return fmt.Errorf("kafka_topic is required when trigger.kind is \"kafka\"")
	}
	return nil
}

// kafkaReader is a minimal interface over kafka.Reader so the consumer can be
// tested with a mock without requiring a live Kafka broker.
// FetchMessage is used instead of ReadMessage so the consumer controls when
// the offset is committed: only after a successful Dispatch, giving
// at-least-once delivery. ReadMessage auto-commits on fetch, which would
// silently drop messages when Dispatch fails.
type kafkaReader interface {
	FetchMessage(ctx context.Context) (kafka.Message, error)
	CommitMessages(ctx context.Context, msgs ...kafka.Message) error
	Close() error
}

type kafkaConsumer struct {
	// newReader is called on each (re)connect attempt, yielding a fresh reader.
	// Using a factory rather than a single reader instance ensures clean state
	// after connection errors — kafka.Reader's internal connection is not reliably
	// reset after certain error types, so recreating it is the safe approach.
	newReader  func() kafkaReader
	workflowID string
	dispatcher Dispatcher
}

func newKafkaConsumer(workflowID, brokers, topic, groupID string, disp Dispatcher) (*kafkaConsumer, error) {
	if err := ValidateKafkaConfig(brokers, topic); err != nil {
		return nil, fmt.Errorf("kafka consumer: %w", err)
	}
	if groupID == "" {
		// All backend replicas join the same consumer group, so Kafka distributes
		// partition ownership across them. On a single-partition topic only one
		// replica receives messages at a time. Set an explicit kafka_group_id in
		// the trigger config to override this default.
		groupID = "cogniflow-" + workflowID
	}
	parts := strings.Split(brokers, ",")
	for i, p := range parts {
		parts[i] = strings.TrimSpace(p)
	}
	return &kafkaConsumer{
		newReader: func() kafkaReader {
			return kafka.NewReader(kafka.ReaderConfig{
				Brokers: parts,
				Topic:   topic,
				GroupID: groupID,
			})
		},
		workflowID: workflowID,
		dispatcher: disp,
	}, nil
}

// run is the outer reconnect loop. It creates a fresh reader on each attempt so
// that connection errors leave no stale state in the reader internals. A panic
// is caught and logged so the Manager's WaitGroup counter is always decremented.
func (c *kafkaConsumer) run(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("kafka trigger: consumer panic; trigger disarmed until next server restart",
				"workflow_id", c.workflowID, "panic", r)
		}
	}()

	backoff := time.Second
	for {
		r := c.newReader()
		reconnect := c.consume(ctx, r, &backoff)
		_ = r.Close()
		if !reconnect || ctx.Err() != nil {
			return
		}
	}
}

// consume reads messages from r until an error or ctx cancellation.
// It uses FetchMessage (manual commit) so that a dispatch failure triggers a
// reconnect and message retry rather than silently advancing the group offset.
// Returns true if the caller should reconnect with a fresh reader.
func (c *kafkaConsumer) consume(ctx context.Context, r kafkaReader, backoff *time.Duration) bool {
	for {
		msg, err := r.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return false
			}
			slog.Error("kafka trigger: read error",
				"workflow_id", c.workflowID, "error", err)
			select {
			case <-ctx.Done():
				return false
			case <-time.After(*backoff):
			}
			*backoff = min(*backoff*2, 30*time.Second)
			return true // signal outer loop to reconnect
		}

		initialData := parseMessageJSON(msg.Value, c.workflowID, "kafka")
		if _, err := c.dispatcher.Dispatch(ctx, RunRequest{
			WorkflowID:  c.workflowID,
			InitialData: initialData,
			TriggeredBy: "kafka",
		}); err != nil {
			slog.Error("kafka trigger: dispatch failed",
				"workflow_id", c.workflowID, "error", err)
			// Back off and reconnect so the uncommitted message is re-delivered.
			select {
			case <-ctx.Done():
				return false
			case <-time.After(*backoff):
			}
			*backoff = min(*backoff*2, 30*time.Second)
			return true
		}
		*backoff = time.Second

		// Commit only after successful dispatch so the message is retried on failure.
		if err := r.CommitMessages(ctx, msg); err != nil {
			slog.Error("kafka trigger: commit failed; message may be reprocessed",
				"workflow_id", c.workflowID, "error", err)
		}
	}
}

// parseMessageJSON decodes raw bytes as a JSON object. On failure it logs a
// warning and returns an empty map so execution still proceeds.
func parseMessageJSON(data []byte, workflowID, kind string) map[string]any {
	if len(data) == 0 {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		slog.Warn("trigger: message body is not valid JSON, using empty initial data",
			"workflow_id", workflowID, "kind", kind, "error", err)
		return map[string]any{}
	}
	return m
}
