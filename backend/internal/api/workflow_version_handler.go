package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/g8rswimmer/cogniflow/internal/store"
)

type workflowVersionHandler struct {
	store    store.Store
	triggers triggerManager
}

// listVersions handles GET /v1/workflows/{id}/versions
func (h *workflowVersionHandler) listVersions(w http.ResponseWriter, r *http.Request) {
	workflowID := r.PathValue("id")
	summaries, err := h.store.ListWorkflowVersions(r.Context(), workflowID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	if summaries == nil {
		summaries = []store.WorkflowVersionSummary{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"versions": summaries})
}

// getVersion handles GET /v1/workflows/{id}/versions/{version_number}
func (h *workflowVersionHandler) getVersion(w http.ResponseWriter, r *http.Request) {
	workflowID := r.PathValue("id")
	versionNum, err := strconv.Atoi(r.PathValue("version_number"))
	if err != nil || versionNum < 1 {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid version_number")
		return
	}

	ver, err := h.store.GetWorkflowVersion(r.Context(), workflowID, versionNum)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "version not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	maskSensitiveConfig(ver.Definition.Nodes)
	writeJSON(w, http.StatusOK, map[string]any{"version": ver})
}

// restoreVersion handles POST /v1/workflows/{id}/versions/{version_number}/restore
func (h *workflowVersionHandler) restoreVersion(w http.ResponseWriter, r *http.Request) {
	workflowID := r.PathValue("id")
	versionNum, err := strconv.Atoi(r.PathValue("version_number"))
	if err != nil || versionNum < 1 {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid version_number")
		return
	}

	restored, err := h.store.RestoreWorkflowVersion(r.Context(), workflowID, versionNum)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "version not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	// Re-arm the trigger manager with the restored trigger config.
	if tErr := h.triggers.Upsert(workflowID, store.TriggerConfig{
		Kind:     restored.Trigger.Kind,
		CronExpr: restored.Trigger.CronExpr,
	}); tErr != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "restore succeeded but trigger update failed: "+tErr.Error())
		return
	}

	maskSensitiveConfig(restored.Nodes)
	writeJSON(w, http.StatusOK, toWorkflowResponse(restored))
}
