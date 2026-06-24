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
	"github.com/g8rswimmer/cogniflow/internal/node/builtin/conditional"
	loopcontroller "github.com/g8rswimmer/cogniflow/internal/node/builtin/loop_controller"
	"github.com/g8rswimmer/cogniflow/internal/node/outputparser"
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

	if verrs := h.collectValidationErrors(&wf); len(verrs) > 0 {
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

	if verrs := h.collectValidationErrors(&wf); len(verrs) > 0 {
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

// collectValidationErrors runs all node/field validators and returns every
// error found across all nodes in a single pass. The create and update handlers
// call this once and respond with the full list so the frontend can highlight
// every problem node simultaneously rather than surfacing one error at a time.
func (h *workflowHandler) collectValidationErrors(wf *store.Workflow) []FieldValidationError {
	var errs []FieldValidationError
	errs = append(errs, h.validateRequiredFields(wf)...)
	errs = append(errs, h.validateTemplates(wf)...)
	errs = append(errs, validateOutputParsers(wf.Nodes)...)
	errs = append(errs, validateCELExpressions(wf.Nodes)...)
	errs = append(errs, validateEdgeBranchLabels(wf.Nodes, wf.Edges)...)
	errs = append(errs, validateLoopEdges(wf.Nodes, wf.Edges)...)
	return errs
}

// validateRequiredFields checks that every field listed in a node's InputSchema
// "required" array is present and non-empty in the node's config.
func (h *workflowHandler) validateRequiredFields(wf *store.Workflow) []FieldValidationError {
	var errs []FieldValidationError
	for _, n := range wf.Nodes {
		handler, err := h.registry.Lookup(n.TypeID)
		if err != nil {
			continue // unknown type caught by validate()
		}
		for _, field := range parseRequiredFields(handler.Meta().InputSchema) {
			v, ok := n.Config[field]
			if !ok || v == nil {
				errs = append(errs, FieldValidationError{NodeID: n.ID, Field: field, Message: "required field is missing"})
				continue
			}
			if s, ok := v.(string); ok && s == "" {
				errs = append(errs, FieldValidationError{NodeID: n.ID, Field: field, Message: "required field must not be empty"})
			}
		}
	}
	return errs
}

// parseRequiredFields extracts the "required" array from a JSON Schema.
func parseRequiredFields(schema json.RawMessage) []string {
	var s struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(schema, &s); err != nil {
		return nil
	}
	return s.Required
}

// validateTemplates parses any x-template:true config field value that contains
// "{{" to catch syntax errors before the workflow is saved.
func (h *workflowHandler) validateTemplates(wf *store.Workflow) []FieldValidationError {
	var errs []FieldValidationError
	for _, n := range wf.Nodes {
		handler, err := h.registry.Lookup(n.TypeID)
		if err != nil {
			continue // unknown type already caught by validate()
		}
		fields := parseTemplateFields(handler.Meta().InputSchema)
		for _, f := range fields {
			if f.isMap {
				m, ok := n.Config[f.key].(map[string]any)
				if !ok {
					continue
				}
				for mapKey, v := range m {
					val, ok := v.(string)
					if !ok || !strings.Contains(val, "{{") {
						continue
					}
					if _, parseErr := template.New("").Parse(val); parseErr != nil {
						errs = append(errs, FieldValidationError{
							NodeID:  n.ID,
							Field:   fmt.Sprintf("%s[%s]", f.key, mapKey),
							Message: "invalid template: " + parseErr.Error(),
						})
					}
				}
			} else {
				val, ok := n.Config[f.key].(string)
				if !ok || !strings.Contains(val, "{{") {
					continue
				}
				if _, parseErr := template.New("").Parse(val); parseErr != nil {
					errs = append(errs, FieldValidationError{
						NodeID:  n.ID,
						Field:   f.key,
						Message: "invalid template: " + parseErr.Error(),
					})
				}
			}
		}
	}
	return errs
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


// validateCELExpressions validates every conditional.branch node at save time.
// Detects old format (config["expression"]) vs new format (config["rules"]) per node.
func validateCELExpressions(nodes []store.WorkflowNode) []FieldValidationError {
	var errs []FieldValidationError
	for _, n := range nodes {
		if n.TypeID != "conditional.branch" {
			continue
		}
		// New format: structured rules key is present.
		if rawRules, hasRules := n.Config["rules"]; hasRules {
			if rawRules == nil {
				errs = append(errs, FieldValidationError{
					NodeID:  n.ID,
					Field:   "rules",
					Message: "rules must not be null; define at least one rule or use a CEL expression",
				})
				continue
			}
			rules, err := parseConditionalRules(rawRules)
			if err != nil {
				errs = append(errs, FieldValidationError{
					NodeID:  n.ID,
					Field:   "rules",
					Message: "invalid rules format: " + err.Error(),
				})
				continue
			}
			if err := conditional.ValidateRules(rules); err != nil {
				errs = append(errs, FieldValidationError{
					NodeID:  n.ID,
					Field:   "rules",
					Message: err.Error(),
				})
			}
			continue
		}
		// Legacy format: raw CEL expression.
		expr, _ := n.Config["expression"].(string)
		if expr == "" {
			errs = append(errs, FieldValidationError{
				NodeID:  n.ID,
				Message: "conditional.branch node must have either a 'rules' array (new format) or a non-empty 'expression' (legacy CEL)",
			})
			continue
		}
		if err := conditional.ValidateExpression(expr); err != nil {
			errs = append(errs, FieldValidationError{
				NodeID:  n.ID,
				Field:   "expression",
				Message: err.Error(),
			})
		}
	}
	return errs
}

// parseConditionalRules round-trips the raw config value through JSON to get
// a typed []conditional.ConditionalRule slice.
func parseConditionalRules(raw any) ([]conditional.ConditionalRule, error) {
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var rules []conditional.ConditionalRule
	if err := json.Unmarshal(b, &rules); err != nil {
		return nil, err
	}
	return rules, nil
}

// validateOutputParsers checks that every output_parser defined on each node
// has a valid kind and pattern.
func validateOutputParsers(nodes []store.WorkflowNode) []FieldValidationError {
	var errs []FieldValidationError
	for _, n := range nodes {
		if err := outputparser.ValidateAll(n.OutputParsers); err != nil {
			errs = append(errs, FieldValidationError{
				NodeID:  n.ID,
				Message: "output parser error: " + err.Error(),
			})
		}
	}
	return errs
}

// validateEdgeBranchLabels validates branch labels on edges from conditional and
// loop.controller nodes.
//   - conditional.branch (new format): labels must match a defined rule label or "fallback".
//   - conditional.branch (legacy format): labels must be "true" or "false".
//   - loop.controller: labels must be "loop_body" or "exit" (on forward, non-loop-back edges).
//   - All other node types: no branch labels permitted.
func validateEdgeBranchLabels(nodes []store.WorkflowNode, edges []store.WorkflowEdge) []FieldValidationError {
	// Build a map of node ID → type for quick lookup.
	nodeTypeOf := make(map[string]string, len(nodes))
	for _, n := range nodes {
		nodeTypeOf[n.ID] = n.TypeID
	}

	// Build a map of conditional node ID → allowed branch labels.
	type nodeFormat struct {
		isNew  bool
		labels map[string]bool // nil means legacy ("true"/"false" only)
	}
	nodeFormats := make(map[string]nodeFormat, len(nodes))
	for _, n := range nodes {
		if n.TypeID != "conditional.branch" {
			continue
		}
		if rawRules, hasRules := n.Config["rules"]; hasRules {
			// Null or malformed rules — validateCELExpressions already reported the error.
			// Treat as new-format with no allowed labels so edge validation is consistent.
			if rawRules == nil {
				nodeFormats[n.ID] = nodeFormat{isNew: true, labels: map[string]bool{"fallback": true}}
				continue
			}
			rules, err := parseConditionalRules(rawRules)
			if err != nil {
				nodeFormats[n.ID] = nodeFormat{isNew: true, labels: map[string]bool{"fallback": true}}
				continue
			}
			allowed := make(map[string]bool, len(rules)+1)
			for _, r := range rules {
				allowed[r.Label] = true
			}
			allowed["fallback"] = true
			nodeFormats[n.ID] = nodeFormat{isNew: true, labels: allowed}
		} else {
			nodeFormats[n.ID] = nodeFormat{isNew: false}
		}
	}

	var errs []FieldValidationError
	for _, e := range edges {
		if e.BranchLabel == nil {
			continue
		}
		if e.IsLoopBack {
			// Loop-back edges must not carry a branch label; validated by validateLoopEdges.
			continue
		}
		label := *e.BranchLabel

		// loop.controller forward edges use "loop_body" and "exit" labels.
		if nodeTypeOf[e.SourceID] == "loop.controller" {
			if label != "loop_body" && label != "exit" {
				errs = append(errs, FieldValidationError{
					NodeID:  e.SourceID,
					Message: fmt.Sprintf("loop.controller edges must be labelled \"loop_body\" or \"exit\", got %q", label),
				})
			}
			continue
		}

		nf, isConditional := nodeFormats[e.SourceID]
		if !isConditional {
			// Only conditional.branch and loop.controller nodes may have labelled edges.
			errs = append(errs, FieldValidationError{
				NodeID:  e.SourceID,
				Message: fmt.Sprintf("branch_label %q on a non-conditional node — only conditional.branch and loop.controller nodes may have labelled edges", label),
			})
			continue
		}
		if nf.isNew {
			if !nf.labels[label] {
				errs = append(errs, FieldValidationError{
					NodeID:  e.SourceID,
					Message: fmt.Sprintf("branch_label %q does not match any rule label or \"fallback\"", label),
				})
			}
		} else {
			if label != "true" && label != "false" {
				errs = append(errs, FieldValidationError{
					NodeID:  e.SourceID,
					Message: fmt.Sprintf("branch_label must be \"true\" or \"false\", got %q", label),
				})
			}
		}
	}
	return errs
}

// validateLoopEdges performs save-time validation of loop-back edges and loop.controller
// node configuration, complementing the structural checks in engine.Build.
func validateLoopEdges(nodes []store.WorkflowNode, edges []store.WorkflowEdge) []FieldValidationError {
	nodeTypeOf := make(map[string]string, len(nodes))
	for _, n := range nodes {
		nodeTypeOf[n.ID] = n.TypeID
	}

	var errs []FieldValidationError

	for _, e := range edges {
		if !e.IsLoopBack {
			continue
		}
		// Loop-back edges must not carry a branch label.
		if e.BranchLabel != nil {
			errs = append(errs, FieldValidationError{
				NodeID:  e.SourceID,
				Message: "loop-back edges must not have a branch_label",
			})
		}
		// Loop-back edges must target a loop.controller node.
		if nodeTypeOf[e.TargetID] != "loop.controller" {
			errs = append(errs, FieldValidationError{
				NodeID:  e.SourceID,
				Message: fmt.Sprintf("loop-back edge must target a loop.controller node, but targets node %q (type %q)", e.TargetID, nodeTypeOf[e.TargetID]),
			})
		}
	}

	// Validate exit_condition CEL on loop.controller nodes.
	for _, n := range nodes {
		if n.TypeID != "loop.controller" {
			continue
		}
		if expr, ok := n.Config["exit_condition"].(string); ok && expr != "" {
			if err := loopcontroller.ValidateExitCondition(expr); err != nil {
				errs = append(errs, FieldValidationError{
					NodeID:  n.ID,
					Field:   "exit_condition",
					Message: err.Error(),
				})
			}
		}
	}

	return errs
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
