package api

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// FieldValidationError describes a single save-time validation failure.
// NodeID and Field are optional — a workflow-level error (e.g. cycle) may
// omit both; a node-level error omits Field.
type FieldValidationError struct {
	NodeID  string `json:"node_id,omitempty"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": message,
			"details": map[string]any{},
		},
	})
}

// writeValidationErrors responds with VALIDATION_FAILED and a structured list
// of per-node / per-field errors so the frontend can highlight specific nodes
// and form fields without parsing the human-readable message string.
func writeValidationErrors(w http.ResponseWriter, errs []FieldValidationError) {
	summary := fmt.Sprintf("Workflow validation failed: %d error(s)", len(errs))
	writeJSON(w, http.StatusBadRequest, map[string]any{
		"error": map[string]any{
			"code":    "VALIDATION_FAILED",
			"message": summary,
			"details": map[string]any{
				"validation_errors": errs,
			},
		},
	})
}
