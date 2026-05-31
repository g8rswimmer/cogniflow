package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/g8rswimmer/cogniflow/internal/store"
	"github.com/g8rswimmer/cogniflow/internal/trigger"
)

type runHandler struct {
	store      store.Store
	dispatcher trigger.Dispatcher
}

// triggerRun handles POST /workflows/{id}/runs.
func (h *runHandler) triggerRun(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	workflowID := r.PathValue("id")

	// Verify the workflow exists.
	if _, err := h.store.GetWorkflow(r.Context(), workflowID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "workflow not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	var body struct {
		InitialData map[string]any `json:"initial_data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		// Allow empty body (no initial data).
		body.InitialData = map[string]any{}
	}
	if body.InitialData == nil {
		body.InitialData = map[string]any{}
	}

	runID, err := h.dispatcher.Dispatch(r.Context(), trigger.RunRequest{
		WorkflowID:  workflowID,
		InitialData: body.InitialData,
		TriggeredBy: "manual",
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ENGINE_ERROR", err.Error())
		return
	}

	// Return 201 with minimal run info; client polls GET /runs/:run_id for status.
	run, err := h.store.GetRun(r.Context(), runID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, run)
}

// getRun handles GET /runs/{run_id}.
func (h *runHandler) getRun(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("run_id")
	run, err := h.store.GetRun(r.Context(), runID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "run not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, run)
}

// listRuns handles GET /workflows/{id}/runs.
func (h *runHandler) listRuns(w http.ResponseWriter, r *http.Request) {
	workflowID := r.PathValue("id")

	filter := store.RunFilter{
		WorkflowID: workflowID,
		Status:     store.RunStatus(r.URL.Query().Get("status")),
		Limit:      50, // default cap
	}
	if l := r.URL.Query().Get("limit"); l != "" {
		var n int
		if _, err := fmt.Sscanf(l, "%d", &n); err == nil && n > 0 {
			filter.Limit = n
		}
	}

	runs, err := h.store.ListRuns(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	if runs == nil {
		runs = []store.Run{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": runs})
}
