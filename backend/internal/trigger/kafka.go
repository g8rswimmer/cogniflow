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
	reader     kafkaReader
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
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers: parts,
		Topic:   topic,
		GroupID: groupID,
	})
	return &kafkaConsumer{
		reader:     r,
		workflowID: workflowID,
		dispatcher: disp,
	}, nil
}

func (c *kafkaConsumer) start(ctx context.Context) {
	go c.run(ctx)
}

func (c *kafkaConsumer) run(ctx context.Context) {
	defer c.reader.Close()
	backoff := time.Second
	for {
		msg, err := c.reader.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Error("kafka trigger: read error",
				"workflow_id", c.workflowID, "error", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, 30*time.Second)
			continue
		}
		backoff = time.Second

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
