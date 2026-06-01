package trigger

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/g8rswimmer/cogniflow/internal/store"
)

// Manager coordinates all trigger types for the running server.
// It arms cron jobs and exposes the webhook HTTP handler. Webhook dispatch
// is handled at request time via a static /webhooks/{workflow_id} route;
// the Manager only manages in-memory cron state.
type Manager struct {
	store      store.Store
	dispatcher Dispatcher
	cron       *cronScheduler
	webhook    *webhookHandler
}

// NewManager creates a Manager. Call LoadAll then Start before serving requests.
func NewManager(st store.Store, disp Dispatcher) *Manager {
	return &Manager{
		store:      st,
		dispatcher: disp,
		cron:       newCronScheduler(),
		webhook:    &webhookHandler{store: st, dispatcher: disp},
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
	m.cron.remove(workflowID) // always deactivate first
	return m.activate(workflowID, cfg)
}

// Remove deactivates the in-memory trigger for workflowID (e.g. after a workflow
// is deleted). It is a no-op if no trigger is registered or if m is nil.
func (m *Manager) Remove(workflowID string) {
	if m == nil {
		return
	}
	m.cron.remove(workflowID)
}

// Start begins the cron scheduler. Call after LoadAll.
func (m *Manager) Start() { m.cron.start() }

// Stop halts the cron scheduler gracefully.
func (m *Manager) Stop() { m.cron.stop() }

// WebhookHandler returns an http.HandlerFunc suitable for POST /webhooks/{workflow_id}.
func (m *Manager) WebhookHandler() http.HandlerFunc { return m.webhook.handle }

func (m *Manager) activate(workflowID string, cfg store.TriggerConfig) error {
	switch cfg.Kind {
	case "cron":
		return m.cron.add(workflowID, cfg.CronExpr, func() {
			if _, err := m.dispatcher.Dispatch(context.Background(), RunRequest{
				WorkflowID:  workflowID,
				TriggeredBy: "cron",
			}); err != nil {
				slog.Error("cron trigger: dispatch failed",
					"workflow_id", workflowID, "error", err)
			}
		})
	case "webhook", "manual", "":
		// Webhook is handled statically at request time; manual is on-demand only.
		return nil
	default:
		return fmt.Errorf("trigger manager: unknown kind %q for workflow %s", cfg.Kind, workflowID)
	}
}
