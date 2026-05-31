package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/g8rswimmer/cogniflow/internal/engine"
	"github.com/g8rswimmer/cogniflow/internal/node"
	"github.com/g8rswimmer/cogniflow/internal/store"
)

type workflowHandler struct {
	store    store.Store
	registry *node.NodeRegistry
}

// list handles GET /workflows
func (h *workflowHandler) list(w http.ResponseWriter, r *http.Request) {
	summaries, err := h.store.ListWorkflows(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	if summaries == nil {
		summaries = []store.WorkflowSummary{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"workflows": summaries})
}

// create handles POST /workflows
func (h *workflowHandler) create(w http.ResponseWriter, r *http.Request) {
	var wf store.Workflow
	if err := json.NewDecoder(r.Body).Decode(&wf); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid request body: "+err.Error())
		return
	}

	if err := h.validate(&wf); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
		return
	}

	if err := engine.CycleDetect(wf.Nodes, wf.Edges); err != nil {
		if errors.Is(err, engine.ErrCycleDetected) {
			writeError(w, http.StatusBadRequest, "CYCLE_DETECTED", err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
		return
	}

	created, err := h.store.CreateWorkflow(r.Context(), wf)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "WORKFLOW_SAVE_FAILED", err.Error())
		return
	}

	h.setWebhookURL(&created)
	maskSensitiveConfig(created.Nodes)
	writeJSON(w, http.StatusCreated, created)
}

// get handles GET /workflows/{id}
func (h *workflowHandler) get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wf, err := h.store.GetWorkflow(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "workflow not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	h.setWebhookURL(&wf)
	maskSensitiveConfig(wf.Nodes)
	writeJSON(w, http.StatusOK, wf)
}

// update handles PUT /workflows/{id}
func (h *workflowHandler) update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var wf store.Workflow
	if err := json.NewDecoder(r.Body).Decode(&wf); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid request body: "+err.Error())
		return
	}
	wf.ID = id

	if err := h.validate(&wf); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
		return
	}

	if err := engine.CycleDetect(wf.Nodes, wf.Edges); err != nil {
		if errors.Is(err, engine.ErrCycleDetected) {
			writeError(w, http.StatusBadRequest, "CYCLE_DETECTED", err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
		return
	}

	updated, err := h.store.UpdateWorkflow(r.Context(), wf)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "workflow not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "WORKFLOW_SAVE_FAILED", err.Error())
		return
	}

	h.setWebhookURL(&updated)
	maskSensitiveConfig(updated.Nodes)
	writeJSON(w, http.StatusOK, updated)
}

// delete handles DELETE /workflows/{id}
func (h *workflowHandler) delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	err := h.store.DeleteWorkflow(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "workflow not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// validate checks required fields.
func (h *workflowHandler) validate(wf *store.Workflow) error {
	if wf.Name == "" {
		return errors.New("name is required")
	}
	if wf.Trigger.Kind == "" {
		wf.Trigger.Kind = "manual"
	}
	return nil
}

// setWebhookURL populates the computed webhook_url for webhook-triggered workflows.
func (h *workflowHandler) setWebhookURL(wf *store.Workflow) {
	if wf.Trigger.Kind == "webhook" {
		wf.Trigger.WebhookURL = "/webhooks/" + wf.ID
	}
}

// maskSensitiveConfig replaces sensitive config values with "***" in API responses.
// It relies on each node's SensitiveKeys map set by the ConfigVault on read.
func maskSensitiveConfig(nodes []store.WorkflowNode) {
	for i := range nodes {
		n := &nodes[i]
		for key, isSensitive := range n.SensitiveKeys {
			if isSensitive {
				if _, ok := n.Config[key]; ok {
					n.Config[key] = "***"
				}
			}
		}
	}
}
