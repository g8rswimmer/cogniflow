package trigger

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

// ValidateSQSConfig checks that the SQS-specific trigger fields are present.
func ValidateSQSConfig(queueURL, region string) error {
	if strings.TrimSpace(queueURL) == "" {
		return fmt.Errorf("sqs_queue_url is required when trigger.kind is \"sqs\"")
	}
	if strings.TrimSpace(region) == "" {
		return fmt.Errorf("sqs_region is required when trigger.kind is \"sqs\"")
	}
	return nil
}

// sqsAPI is a minimal interface over sqs.Client so the consumer can be tested
// with a mock without requiring real AWS credentials.
type sqsAPI interface {
	ReceiveMessage(ctx context.Context, params *sqs.ReceiveMessageInput, optFns ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error)
	DeleteMessage(ctx context.Context, params *sqs.DeleteMessageInput, optFns ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error)
}

type sqsConsumer struct {
	client     sqsAPI
	queueURL   string
	workflowID string
	dispatcher Dispatcher
}

func newSQSConsumer(workflowID, queueURL, region string, disp Dispatcher) (*sqsConsumer, error) {
	if err := ValidateSQSConfig(queueURL, region); err != nil {
		return nil, fmt.Errorf("sqs consumer: %w", err)
	}
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(region),
	)
	if err != nil {
		return nil, fmt.Errorf("sqs consumer: load AWS config: %w", err)
	}
	return &sqsConsumer{
		client:     sqs.NewFromConfig(cfg),
		queueURL:   queueURL,
		workflowID: workflowID,
		dispatcher: disp,
	}, nil
}

func (c *sqsConsumer) run(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("sqs trigger: consumer panic; trigger disarmed until next server restart",
				"workflow_id", c.workflowID, "panic", r)
		}
	}()

	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		out, err := c.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            aws.String(c.queueURL),
			MaxNumberOfMessages: 10,
			WaitTimeSeconds:     20,
		})
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Error("sqs trigger: receive error",
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

		for _, msg := range out.Messages {
			c.handleMessage(ctx, msg)
		}
	}
}

func (c *sqsConsumer) handleMessage(ctx context.Context, msg types.Message) {
	body := aws.ToString(msg.Body)
	initialData := parseMessageJSON([]byte(body), c.workflowID, "sqs")

	if _, err := c.dispatcher.Dispatch(ctx, RunRequest{
		WorkflowID:  c.workflowID,
		InitialData: initialData,
		TriggeredBy: "sqs",
	}); err != nil {
		slog.Error("sqs trigger: dispatch failed",
			"workflow_id", c.workflowID, "error", err)
		// Do not delete the message; SQS will re-deliver after the visibility timeout.
		return
	}

	// Use a short-lived independent context for DeleteMessage so that lifecycle
	// cancellation (Manager.Stop) cannot prevent cleanup of a successfully
	// dispatched message — which would otherwise cause a duplicate run.
	delCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := c.client.DeleteMessage(delCtx, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(c.queueURL),
		ReceiptHandle: msg.ReceiptHandle,
	}); err != nil {
		slog.Warn("sqs trigger: failed to delete message after dispatch",
			"workflow_id", c.workflowID, "error", err)
	}
}
