package trigger

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/g8rswimmer/cogniflow/internal/store"
)

// triggerConfigGetter is the only store capability the webhook handler needs.
// Defined here (consumer side) per the project's interface convention.
type triggerConfigGetter interface {
	GetTriggerConfig(ctx context.Context, workflowID string) (store.TriggerConfig, error)
}

type webhookHandler struct {
	store      triggerConfigGetter
	dispatcher Dispatcher
}

// handle is the HTTP handler for POST /webhooks/{workflow_id}.
// It verifies the workflow exists and has a webhook trigger, then dispatches a run.
func (h *webhookHandler) handle(w http.ResponseWriter, r *http.Request) {
	workflowID := r.PathValue("workflow_id")

	cfg, err := h.store.GetTriggerConfig(r.Context(), workflowID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeWebhookError(w, http.StatusNotFound, "NOT_FOUND", "workflow not found")
			return
		}
		writeWebhookError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	if cfg.Kind != "webhook" {
		writeWebhookError(w, http.StatusBadRequest, "INVALID_TRIGGER",
			"workflow does not have a webhook trigger")
		return
	}

	var initialData map[string]any
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&initialData); err != nil && !errors.Is(err, io.EOF) {
			writeWebhookError(w, http.StatusBadRequest, "INVALID_BODY",
				"request body must be a JSON object or empty")
			return
		}
	}
	if initialData == nil {
		initialData = map[string]any{}
	}

	runID, err := h.dispatcher.Dispatch(r.Context(), RunRequest{
		WorkflowID:  workflowID,
		InitialData: initialData,
		TriggeredBy: "webhook",
	})
	if err != nil {
		slog.Error("webhook: dispatch failed", "workflow_id", workflowID, "error", err)
		writeWebhookError(w, http.StatusInternalServerError, "ENGINE_ERROR", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{"run_id": runID})
}

func writeWebhookError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{"code": code, "message": message},
	})
}
