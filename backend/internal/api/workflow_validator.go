package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	"github.com/g8rswimmer/cogniflow/internal/node"
	"github.com/g8rswimmer/cogniflow/internal/node/builtin/conditional"
	loopcontroller "github.com/g8rswimmer/cogniflow/internal/node/builtin/loop_controller"
	"github.com/g8rswimmer/cogniflow/internal/node/outputparser"
	"github.com/g8rswimmer/cogniflow/internal/store"
)

// collectValidationErrors runs all node/field validators and returns every
// error found in a single pass. Handlers call this once and respond with the
// full list so the frontend can highlight every problem node simultaneously.
func collectValidationErrors(wf *store.Workflow, registry *node.NodeRegistry) []FieldValidationError {
	var errs []FieldValidationError
	errs = append(errs, validateRequiredFields(wf, registry)...)
	errs = append(errs, validateTemplates(wf, registry)...)
	errs = append(errs, validateOutputParsers(wf.Nodes)...)
	errs = append(errs, validateCELExpressions(wf.Nodes)...)
	errs = append(errs, validateEdgeBranchLabels(wf.Nodes, wf.Edges)...)
	errs = append(errs, validateLoopEdges(wf.Nodes, wf.Edges)...)
	return errs
}

// validateRequiredFields checks that every field listed in a node's InputSchema
// "required" array is present and non-empty in the node's config.
func validateRequiredFields(wf *store.Workflow, registry *node.NodeRegistry) []FieldValidationError {
	var errs []FieldValidationError
	for _, n := range wf.Nodes {
		handler, err := registry.Lookup(n.TypeID)
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
func validateTemplates(wf *store.Workflow, registry *node.NodeRegistry) []FieldValidationError {
	var errs []FieldValidationError
	for _, n := range wf.Nodes {
		handler, err := registry.Lookup(n.TypeID)
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
	nodeTypeOf := make(map[string]string, len(nodes))
	for _, n := range nodes {
		nodeTypeOf[n.ID] = n.TypeID
	}

	type nodeFormat struct {
		isNew  bool
		labels map[string]bool
	}
	nodeFormats := make(map[string]nodeFormat, len(nodes))
	for _, n := range nodes {
		if n.TypeID != "conditional.branch" {
			continue
		}
		if rawRules, hasRules := n.Config["rules"]; hasRules {
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
			continue
		}
		label := *e.BranchLabel

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
		if e.BranchLabel != nil {
			errs = append(errs, FieldValidationError{
				NodeID:  e.SourceID,
				Message: "loop-back edges must not have a branch_label",
			})
		}
		if nodeTypeOf[e.TargetID] != "loop.controller" {
			errs = append(errs, FieldValidationError{
				NodeID:  e.SourceID,
				Message: fmt.Sprintf("loop-back edge must target a loop.controller node, but targets node %q (type %q)", e.TargetID, nodeTypeOf[e.TargetID]),
			})
		}
	}

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
