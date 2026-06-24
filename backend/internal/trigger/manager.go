package trigger

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"github.com/g8rswimmer/cogniflow/internal/store"
)

// Manager coordinates all trigger types for the running server.
// It arms cron jobs, Kafka consumers, and SQS consumers, and exposes the
// webhook HTTP handler. Webhook dispatch is handled at request time via a
// static /webhooks/{workflow_id} route.
type Manager struct {
	store      store.Store
	dispatcher Dispatcher
	cron       *cronScheduler
	webhook    *webhookHandler
	done       chan struct{} // closed by Stop() to signal shutdown to in-flight cron dispatches

	mu        sync.Mutex
	consumers map[string]func() // workflowID → cancel func for Kafka/SQS goroutines
	wg        sync.WaitGroup   // tracks running consumer goroutines for clean shutdown
}

// NewManager creates a Manager. Call LoadAll then Start before serving requests.
func NewManager(st store.Store, disp Dispatcher) *Manager {
	return &Manager{
		store:      st,
		dispatcher: disp,
		cron:       newCronScheduler(),
		webhook:    &webhookHandler{store: st, dispatcher: disp},
		done:       make(chan struct{}),
		consumers:  make(map[string]func()),
	}
}

// LoadAll reads all non-manual trigger configs from the database and activates
// them. Call this once at startup, before Start.
func (m *Manager) LoadAll(ctx context.Context) error {
	triggers, err := m.store.ListTriggerConfigs(ctx)
	if err != nil {
		return fmt.Errorf("trigger manager: load all: %w", err)
	}
	for _, wt := range triggers {
		if err := m.activate(wt.WorkflowID, wt.Config); err != nil {
			slog.Warn("trigger manager: could not activate trigger at startup",
				"workflow_id", wt.WorkflowID,
				"kind", wt.Config.Kind,
				"error", err,
			)
		}
	}
	return nil
}

// Upsert arms or disarms the in-memory trigger for workflowID based on cfg.
// The caller is responsible for persisting cfg to the store (UpdateWorkflow does this).
// Safe to call on a nil *Manager (no-op).
func (m *Manager) Upsert(workflowID string, cfg store.TriggerConfig) error {
	if m == nil {
		return nil
	}
	return m.activate(workflowID, cfg)
}

// Remove deactivates the in-memory trigger for workflowID (e.g. after a workflow
// is deleted). It is a no-op if no trigger is registered or if m is nil.
func (m *Manager) Remove(workflowID string) {
	if m == nil {
		return
	}
	m.cron.remove(workflowID)
	m.stopConsumer(workflowID)
}

// Start begins the cron scheduler. Call after LoadAll.
func (m *Manager) Start() { m.cron.start() }

// Stop signals shutdown to any in-flight cron dispatches, halts the scheduler,
// and waits for all running cron job goroutines to finish before returning.
// It also cancels all active Kafka and SQS consumer goroutines.
func (m *Manager) Stop() {
	close(m.done)
	drainCtx := m.cron.stop()
	<-drainCtx.Done()

	m.mu.Lock()
	for id, cancel := range m.consumers {
		cancel()
		delete(m.consumers, id)
	}
	m.mu.Unlock()

	m.wg.Wait()
}

// stopConsumer cancels and removes the consumer goroutine for workflowID, if any.
func (m *Manager) stopConsumer(workflowID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cancel, ok := m.consumers[workflowID]; ok {
		cancel()
		delete(m.consumers, workflowID)
	}
}

// setConsumer replaces any existing consumer for workflowID with cancel.
func (m *Manager) setConsumer(workflowID string, cancel func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if old, ok := m.consumers[workflowID]; ok {
		old()
	}
	m.consumers[workflowID] = cancel
}

// WebhookHandler returns an http.HandlerFunc suitable for POST /webhooks/{workflow_id}.
func (m *Manager) WebhookHandler() http.HandlerFunc { return m.webhook.handle }

func (m *Manager) activate(workflowID string, cfg store.TriggerConfig) error {
	switch cfg.Kind {
	case "cron":
		// Stop any running consumer before arming cron.
		m.stopConsumer(workflowID)
		// cron.add atomically removes any existing job for workflowID before
		// scheduling the new one, so no pre-remove is needed here.
		done := m.done
		return m.cron.add(workflowID, cfg.CronExpr, func() {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			// Mirror the done signal into the per-fire context so that a
			// shutdown in progress cancels the Dispatch call.
			go func() {
				select {
				case <-done:
					cancel()
				case <-ctx.Done():
				}
			}()
			if _, err := m.dispatcher.Dispatch(ctx, RunRequest{
				WorkflowID:  workflowID,
				TriggeredBy: "cron",
			}); err != nil {
				slog.Error("cron trigger: dispatch failed",
					"workflow_id", workflowID, "error", err)
			}
		})
	case "kafka":
		m.cron.remove(workflowID)
		consumer, err := newKafkaConsumer(workflowID, cfg.KafkaBrokers, cfg.KafkaTopic, cfg.KafkaGroupID, m.dispatcher)
		if err != nil {
			return fmt.Errorf("trigger manager: kafka: %w", err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		m.setConsumer(workflowID, cancel)
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			consumer.run(ctx)
		}()
		return nil
	case "sqs":
		m.cron.remove(workflowID)
		consumer, err := newSQSConsumer(workflowID, cfg.SQSQueueURL, cfg.SQSRegion, m.dispatcher)
		if err != nil {
			return fmt.Errorf("trigger manager: sqs: %w", err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		m.setConsumer(workflowID, cancel)
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			consumer.run(ctx)
		}()
		return nil
	case "webhook", "manual", "":
		// If the workflow previously had a cron trigger or consumer, disarm it.
		m.cron.remove(workflowID)
		m.stopConsumer(workflowID)
		// Webhook is handled statically at request time; manual is on-demand only.
		return nil
	default:
		return fmt.Errorf("trigger manager: unknown kind %q for workflow %s", cfg.Kind, workflowID)
	}
}
