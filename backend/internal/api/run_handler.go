package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

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

	// Verify the workflow exists and capture it for schema validation.
	wf, err := h.store.GetWorkflow(r.Context(), workflowID)
	if err != nil {
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
		if !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid request body: "+err.Error())
			return
		}
	}
	if body.InitialData == nil {
		body.InitialData = map[string]any{}
	}

	// Advisory schema validation: log warnings but never block the run.
	if len(wf.InitialDataSchema) > 0 {
		if warnings := validateInitialData(wf.InitialDataSchema, body.InitialData); len(warnings) > 0 {
			slog.Warn("run trigger: initial_data does not match declared schema",
				"workflow_id", workflowID,
				"warnings", warnings,
			)
		}
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

	if _, err := h.store.GetWorkflow(r.Context(), workflowID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "workflow not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	filter := store.RunFilter{
		WorkflowID: workflowID,
		Status:     store.RunStatus(r.URL.Query().Get("status")),
		Limit:      50, // default cap
	}
	q := r.URL.Query()
	if l := q.Get("limit"); l != "" {
		var n int
		if _, err := fmt.Sscanf(l, "%d", &n); err == nil && n > 0 {
			filter.Limit = n
		}
	}
	if s := q.Get("since"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			filter.Since = t
		}
	}
	if s := q.Get("until"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			filter.Until = t
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

// validateInitialData performs advisory validation of run initial data against the
// workflow's declared schema. It returns warning strings for missing or mistyped
// fields but never returns an error — callers must not block execution on warnings.
func validateInitialData(schema json.RawMessage, data map[string]any) []string {
	var s struct {
		Properties map[string]struct {
			Type string `json:"type"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(schema, &s); err != nil {
		return nil
	}
	var warnings []string
	for fieldName, fieldSchema := range s.Properties {
		val, present := data[fieldName]
		if !present {
			warnings = append(warnings, fmt.Sprintf("declared field %q is missing from initial_data", fieldName))
			continue
		}
		// Lightweight type check: only flag clear mismatches.
		switch fieldSchema.Type {
		case "number", "integer":
			switch val.(type) {
			case float64, int, int64:
				// ok
			default:
				warnings = append(warnings, fmt.Sprintf("field %q declared as %q but got %T", fieldName, fieldSchema.Type, val))
			}
		case "boolean":
			if _, ok := val.(bool); !ok {
				warnings = append(warnings, fmt.Sprintf("field %q declared as %q but got %T", fieldName, fieldSchema.Type, val))
			}
		}
	}
	return warnings
}
