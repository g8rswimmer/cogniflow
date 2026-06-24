package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/g8rswimmer/cogniflow/internal/store"
)

type workflowVersionHandler struct {
	store    store.Store
	triggers triggerManager
}

// workflowVersionResponse is the response shape for a single version, with the
// definition rendered through toWorkflowResponse so webhook_url is included and
// nil nodes/edges are normalised to empty arrays.
type workflowVersionResponse struct {
	ID            string           `json:"id"`
	WorkflowID    string           `json:"workflow_id"`
	VersionNumber int              `json:"version_number"`
	Definition    workflowResponse `json:"definition"`
	CreatedAt     time.Time        `json:"created_at"`
}

// listVersions handles GET /v1/workflows/{id}/versions
func (h *workflowVersionHandler) listVersions(w http.ResponseWriter, r *http.Request) {
	workflowID := r.PathValue("id")

	// Verify the workflow exists; return 404 rather than an empty list for unknown IDs.
	if _, err := h.store.GetWorkflow(r.Context(), workflowID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "workflow not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

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
	writeJSON(w, http.StatusOK, map[string]any{"version": workflowVersionResponse{
		ID:            ver.ID,
		WorkflowID:    ver.WorkflowID,
		VersionNumber: ver.VersionNumber,
		Definition:    toWorkflowResponse(ver.Definition),
		CreatedAt:     ver.CreatedAt,
	}})
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

	// Re-arm the trigger manager with the restored trigger config. The restore is
	// already committed to the DB at this point, so a trigger failure is logged as
	// a warning rather than returning 500 (which would cause clients to retry and
	// create a duplicate version snapshot). The scheduler will reload the correct
	// trigger from the DB on the next server restart.
	if tErr := h.triggers.Upsert(workflowID, triggerConfigFromWorkflow(restored.Trigger)); tErr != nil {
		slog.WarnContext(r.Context(), "version restore: trigger re-arm failed (restore committed)",
			"workflow_id", workflowID, "version", versionNum, "error", tErr)
	}

	maskSensitiveConfig(restored.Nodes)
	writeJSON(w, http.StatusOK, toWorkflowResponse(restored))
}
