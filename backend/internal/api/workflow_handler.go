package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"text/template"
	"time"

	"github.com/g8rswimmer/cogniflow/internal/engine"
	"github.com/g8rswimmer/cogniflow/internal/node"
	"github.com/g8rswimmer/cogniflow/internal/store"
	"github.com/g8rswimmer/cogniflow/internal/trigger"
)

// triggerManager is the subset of trigger.Manager that the workflow handler
// needs. Defined here (consumer) so tests can substitute a lightweight stub
// without importing the full trigger package.
type triggerManager interface {
	Upsert(workflowID string, cfg store.TriggerConfig) error
	Remove(workflowID string)
}

const maxBodyBytes = 1 << 20 // 1 MB

type workflowHandler struct {
	store    store.Store
	registry *node.NodeRegistry
	triggers triggerManager
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
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	var wf store.Workflow
	if err := json.NewDecoder(r.Body).Decode(&wf); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid request body: "+err.Error())
		return
	}

	applyDefaults(&wf)

	if err := h.validate(&wf); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
		return
	}

	if err := h.validateTemplates(&wf); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
		return
	}

	if wf.Trigger.Kind == "cron" {
		if err := trigger.ValidateCronExpr(wf.Trigger.CronExpr); err != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
			return
		}
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

	if err := h.triggers.Upsert(created.ID, store.TriggerConfig{
		Kind:     created.Trigger.Kind,
		CronExpr: created.Trigger.CronExpr,
	}); err != nil {
		slog.Error("workflow create: trigger activation failed", "workflow_id", created.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "TRIGGER_ACTIVATION_FAILED", err.Error())
		return
	}

	maskSensitiveConfig(created.Nodes)
	writeJSON(w, http.StatusCreated, toWorkflowResponse(created))
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

	maskSensitiveConfig(wf.Nodes)
	writeJSON(w, http.StatusOK, toWorkflowResponse(wf))
}

// update handles PUT /workflows/{id}
func (h *workflowHandler) update(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	id := r.PathValue("id")

	var wf store.Workflow
	if err := json.NewDecoder(r.Body).Decode(&wf); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid request body: "+err.Error())
		return
	}
	wf.ID = id

	applyDefaults(&wf)

	if err := h.validate(&wf); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
		return
	}

	if err := h.validateTemplates(&wf); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
		return
	}

	if wf.Trigger.Kind == "cron" {
		if err := trigger.ValidateCronExpr(wf.Trigger.CronExpr); err != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
			return
		}
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

	if err := h.triggers.Upsert(updated.ID, store.TriggerConfig{
		Kind:     updated.Trigger.Kind,
		CronExpr: updated.Trigger.CronExpr,
	}); err != nil {
		slog.Error("workflow update: trigger activation failed", "workflow_id", updated.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "TRIGGER_ACTIVATION_FAILED", err.Error())
		return
	}

	maskSensitiveConfig(updated.Nodes)
	writeJSON(w, http.StatusOK, toWorkflowResponse(updated))
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
	h.triggers.Remove(id)
	w.WriteHeader(http.StatusNoContent)
}

// applyDefaults fills in optional fields before validation.
func applyDefaults(wf *store.Workflow) {
	if wf.Trigger.Kind == "" {
		wf.Trigger.Kind = "manual"
	}
}

// validate rejects requests that are missing required fields or reference unknown node types.
func (h *workflowHandler) validate(wf *store.Workflow) error {
	if wf.Name == "" {
		return errors.New("name is required")
	}
	for _, n := range wf.Nodes {
		if _, err := h.registry.Lookup(n.TypeID); err != nil {
			return fmt.Errorf("unknown node type_id %q", n.TypeID)
		}
	}
	return nil
}

// validateTemplates parses any x-template:true config field value that contains
// "{{" to catch syntax errors before the workflow is saved. This mirrors the
// CEL validation pattern used by the Conditional node.
func (h *workflowHandler) validateTemplates(wf *store.Workflow) error {
	for _, n := range wf.Nodes {
		handler, err := h.registry.Lookup(n.TypeID)
		if err != nil {
			continue // unknown type already caught by validate()
		}
		fields := parseTemplateFields(handler.Meta().InputSchema)
		for _, f := range fields {
			if f.isMap {
				// e.g. headers: map[string]string where each value may be a template.
				m, ok := n.Config[f.key].(map[string]any)
				if !ok {
					continue
				}
				for mapKey, v := range m {
					val, ok := v.(string)
					if !ok || !strings.Contains(val, "{{") {
						continue
					}
					if _, err := template.New("").Parse(val); err != nil {
						return fmt.Errorf("node %q field %q[%q]: invalid template: %w", n.ID, f.key, mapKey, err)
					}
				}
			} else {
				val, ok := n.Config[f.key].(string)
				if !ok || !strings.Contains(val, "{{") {
					continue
				}
				if _, err := template.New("").Parse(val); err != nil {
					return fmt.Errorf("node %q field %q: invalid template: %w", n.ID, f.key, err)
				}
			}
		}
	}
	return nil
}

// templateField describes a single config key whose value(s) may contain Go templates.
type templateField struct {
	key   string
	isMap bool // true when the field value is map[string]any with template string values
}

// parseTemplateFields returns fields in schema marked "x-template":true, including
// fields whose additionalProperties carry the marker (e.g. the headers map).
func parseTemplateFields(schema json.RawMessage) []templateField {
	var s struct {
		Properties map[string]struct {
			XTemplate            bool `json:"x-template"`
			AdditionalProperties *struct {
				XTemplate bool `json:"x-template"`
			} `json:"additionalProperties"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(schema, &s); err != nil {
		return nil
	}
	var fields []templateField
	for k, prop := range s.Properties {
		if prop.XTemplate {
			fields = append(fields, templateField{key: k, isMap: false})
		} else if prop.AdditionalProperties != nil && prop.AdditionalProperties.XTemplate {
			fields = append(fields, templateField{key: k, isMap: true})
		}
	}
	return fields
}

// ---- response types ------------------------------------------------------

// workflowResponse is the API representation of a workflow. It keeps
// WebhookURL in the API layer rather than the store domain type.
type workflowResponse struct {
	ID             string               `json:"id"`
	Name           string               `json:"name"`
	Description    string               `json:"description,omitempty"`
	Trigger        triggerResponse      `json:"trigger"`
	TimeoutSeconds int                  `json:"timeout_seconds"`
	Nodes          []store.WorkflowNode `json:"nodes"`
	Edges          []store.WorkflowEdge `json:"edges"`
	CreatedAt      time.Time            `json:"created_at"`
	UpdatedAt      time.Time            `json:"updated_at"`
}

type triggerResponse struct {
	Kind       string `json:"kind"`
	CronExpr   string `json:"cron_expr,omitempty"`
	WebhookURL string `json:"webhook_url,omitempty"`
}

func toWorkflowResponse(wf store.Workflow) workflowResponse {
	nodes := wf.Nodes
	if nodes == nil {
		nodes = []store.WorkflowNode{}
	}
	edges := wf.Edges
	if edges == nil {
		edges = []store.WorkflowEdge{}
	}

	resp := workflowResponse{
		ID:             wf.ID,
		Name:           wf.Name,
		Description:    wf.Description,
		TimeoutSeconds: wf.TimeoutSeconds,
		Trigger: triggerResponse{
			Kind:     wf.Trigger.Kind,
			CronExpr: wf.Trigger.CronExpr,
		},
		Nodes:     nodes,
		Edges:     edges,
		CreatedAt: wf.CreatedAt,
		UpdatedAt: wf.UpdatedAt,
	}
	if wf.Trigger.Kind == "webhook" {
		resp.Trigger.WebhookURL = "/webhooks/" + wf.ID
	}
	return resp
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
