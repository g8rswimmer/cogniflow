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
type kafkaReader interface {
	ReadMessage(ctx context.Context) (kafka.Message, error)
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

func (c *kafkaConsumer) start(ctx context.Context) {
	go c.run(ctx)
}

// run is the outer reconnect loop. It creates a fresh reader on each attempt so
// that connection errors leave no stale state in the reader internals.
func (c *kafkaConsumer) run(ctx context.Context) {
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
// Returns true if the caller should reconnect with a fresh reader.
func (c *kafkaConsumer) consume(ctx context.Context, r kafkaReader, backoff *time.Duration) bool {
	for {
		msg, err := r.ReadMessage(ctx)
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
		*backoff = time.Second

		initialData := parseMessageJSON(msg.Value, c.workflowID, "kafka")
		if _, err := c.dispatcher.Dispatch(ctx, RunRequest{
			WorkflowID:  c.workflowID,
			InitialData: initialData,
			TriggeredBy: "kafka",
		}); err != nil {
			slog.Error("kafka trigger: dispatch failed",
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
