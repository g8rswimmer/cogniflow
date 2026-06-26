package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
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

	if verrs := collectValidationErrors(&wf, h.registry); len(verrs) > 0 {
		slog.Warn("workflow create: validation failed", "error_count", len(verrs), "errors", verrs)
		writeValidationErrors(w, verrs)
		return
	}

	if err := validateTrigger(wf.Trigger); err != nil {
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

	if err := h.triggers.Upsert(created.ID, triggerConfigFromWorkflow(created.Trigger)); err != nil {
		slog.Error("workflow create: trigger activation failed", "workflow_id", created.ID, "error", err)
		// Roll back the persisted workflow so the client's failed request leaves no orphan.
		// If the delete also fails, log it — the workflow remains in the DB but with no
		// armed trigger and will be re-armed on the next server restart via LoadAll.
		if delErr := h.store.DeleteWorkflow(r.Context(), created.ID); delErr != nil {
			slog.Error("workflow create: rollback failed after trigger error",
				"workflow_id", created.ID, "error", delErr)
		}
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

	if verrs := collectValidationErrors(&wf, h.registry); len(verrs) > 0 {
		slog.Warn("workflow update: validation failed", "workflow_id", id, "error_count", len(verrs), "errors", verrs)
		writeValidationErrors(w, verrs)
		return
	}

	if err := validateTrigger(wf.Trigger); err != nil {
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

	if err := h.triggers.Upsert(updated.ID, triggerConfigFromWorkflow(updated.Trigger)); err != nil {
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

// ---- response types ------------------------------------------------------

// workflowResponse is the API representation of a workflow. It keeps
// WebhookURL in the API layer rather than the store domain type.
type workflowResponse struct {
	ID                string               `json:"id"`
	Name              string               `json:"name"`
	Description       string               `json:"description,omitempty"`
	Trigger           triggerResponse      `json:"trigger"`
	TimeoutSeconds    int                  `json:"timeout_seconds"`
	Nodes             []store.WorkflowNode `json:"nodes"`
	Edges             []store.WorkflowEdge `json:"edges"`
	InitialDataSchema json.RawMessage      `json:"initial_data_schema,omitempty"`
	CreatedAt         time.Time            `json:"created_at"`
	UpdatedAt         time.Time            `json:"updated_at"`
}

type triggerResponse struct {
	Kind         string `json:"kind"`
	CronExpr     string `json:"cron_expr,omitempty"`
	WebhookURL   string `json:"webhook_url,omitempty"`
	KafkaBrokers string `json:"kafka_brokers,omitempty"`
	KafkaTopic   string `json:"kafka_topic,omitempty"`
	KafkaGroupID string `json:"kafka_group_id,omitempty"`
	SQSQueueURL  string `json:"sqs_queue_url,omitempty"`
	SQSRegion    string `json:"sqs_region,omitempty"`
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
		ID:                wf.ID,
		Name:              wf.Name,
		Description:       wf.Description,
		TimeoutSeconds:    wf.TimeoutSeconds,
		InitialDataSchema: wf.InitialDataSchema,
		Trigger: triggerResponse{
			Kind:         wf.Trigger.Kind,
			CronExpr:     wf.Trigger.CronExpr,
			KafkaBrokers: wf.Trigger.KafkaBrokers,
			KafkaTopic:   wf.Trigger.KafkaTopic,
			KafkaGroupID: wf.Trigger.KafkaGroupID,
			SQSQueueURL:  wf.Trigger.SQSQueueURL,
			SQSRegion:    wf.Trigger.SQSRegion,
		},
		Nodes:     nodes,
		Edges:     edges,
		CreatedAt: wf.CreatedAt,
		UpdatedAt: wf.UpdatedAt,
	}
	if wf.Trigger.Kind == "webhook" {
		resp.Trigger.WebhookURL = "/v1/webhooks/" + wf.ID
	}
	return resp
}

// validateTrigger runs kind-specific validation on a workflow trigger config.
func validateTrigger(t store.Trigger) error {
	switch t.Kind {
	case "cron":
		return trigger.ValidateCronExpr(t.CronExpr)
	case "kafka":
		return trigger.ValidateKafkaConfig(t.KafkaBrokers, t.KafkaTopic)
	case "sqs":
		return trigger.ValidateSQSConfig(t.SQSQueueURL, t.SQSRegion)
	default:
		return nil
	}
}

// triggerConfigFromWorkflow maps a store.Trigger to the store.TriggerConfig
// used by the trigger Manager.
func triggerConfigFromWorkflow(t store.Trigger) store.TriggerConfig {
	return store.TriggerConfig(t)
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
