// Package conditional provides the conditional.branch built-in node for cogniflow.
package conditional

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/google/cel-go/cel"

	"github.com/g8rswimmer/cogniflow/internal/node"
)

// ConditionalCondition is one comparison in a rule.
type ConditionalCondition struct {
	NodeID    string `json:"node_id"`    // upstream node whose output to inspect
	Field     string `json:"field"`      // field name in that node's output map
	Operator  string `json:"operator"`   // "==" | "!=" | ">" | ">=" | "<" | "<=" | "contains"
	Value     string `json:"value"`      // right-hand operand, stored as string
	ValueType string `json:"value_type"` // "string" | "number" | "boolean"
}

// ConditionalRule is a named branch with one or more conditions.
type ConditionalRule struct {
	Label      string                 `json:"label"`      // unique per node; "fallback" is reserved
	Logic      string                 `json:"logic"`      // "AND" | "OR"
	Conditions []ConditionalCondition `json:"conditions"` // at least one required
}

var validOperators = []string{"==", "!=", ">", ">=", "<", "<=", "contains"}

var inputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "expression": {
      "type": "string",
      "title": "CEL Expression (legacy)",
      "description": "Legacy: single raw CEL boolean expression. New workflows use the 'rules' field instead."
    },
    "rules": {
      "type": "array",
      "title": "Rules",
      "description": "Ordered list of named conditional rules. First match wins. Fallback fires when none match."
    }
  }
}`)

var outputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "matched_rule": { "type": "string",  "description": "Label of the first matching rule, or \"fallback\" when none matched (new format)" },
    "result":       { "type": "boolean", "description": "Evaluated result for legacy expression-based nodes (true/false)" }
  }
}`)

// Handler implements the conditional.branch node type.
// The compiled CEL program is cached per expression string so that the
// expensive env construction and parse/type-check happen only once across
// all executions of a workflow containing this node.
type Handler struct {
	programs sync.Map // string → cel.Program
}

// New returns a Handler for the "conditional.branch" node type.
func New() *Handler { return &Handler{} }

func (h *Handler) Meta() node.NodeMeta {
	return node.NodeMeta{
		TypeID:       "conditional.branch",
		DisplayName:  "Conditional Branch",
		Category:     "control",
		Description:  "Routes execution to named branches using visual rules (field + operator + value). Supports multiple branches and compound AND/OR conditions. A fallback edge fires when no rule matches.",
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
	}
}

// ValidateExpression compiles the CEL expression and verifies it returns bool (or dyn).
// Called at workflow save time for legacy-format nodes.
func ValidateExpression(expr string) error {
	env, err := newEnv()
	if err != nil {
		return fmt.Errorf("conditional.branch: create CEL env: %w", err)
	}
	ast, iss := env.Compile(expr)
	if iss.Err() != nil {
		return fmt.Errorf("invalid CEL expression: %w", iss.Err())
	}
	outName := ast.OutputType().TypeName()
	if outName != "bool" && outName != "dyn" {
		return fmt.Errorf("CEL expression must return bool, got %s", outName)
	}
	return nil
}

// ValidateRules validates the structured rule list at workflow save time.
func ValidateRules(rules []ConditionalRule) error {
	if len(rules) == 0 {
		return fmt.Errorf("at least one rule is required")
	}
	seen := make(map[string]bool, len(rules))
	for i, r := range rules {
		if r.Label == "" {
			return fmt.Errorf("rule %d: label is required", i)
		}
		if r.Label == "fallback" {
			return fmt.Errorf("rule %d: label \"fallback\" is reserved", i)
		}
		if seen[r.Label] {
			return fmt.Errorf("rule %d: duplicate label %q", i, r.Label)
		}
		seen[r.Label] = true

		if len(r.Conditions) == 0 {
			return fmt.Errorf("rule %q: at least one condition is required", r.Label)
		}
		if r.Logic != "AND" && r.Logic != "OR" {
			return fmt.Errorf("rule %q: logic must be \"AND\" or \"OR\", got %q", r.Label, r.Logic)
		}
		for j, c := range r.Conditions {
			if !slices.Contains(validOperators, c.Operator) {
				return fmt.Errorf("rule %q condition %d: unknown operator %q", r.Label, j, c.Operator)
			}
			if c.NodeID == "" {
				return fmt.Errorf("rule %q condition %d: node_id is required", r.Label, j)
			}
			if c.Field == "" {
				return fmt.Errorf("rule %q condition %d: field is required", r.Label, j)
			}
			if c.ValueType != "string" && c.ValueType != "number" && c.ValueType != "boolean" {
				return fmt.Errorf("rule %q condition %d: value_type must be \"string\", \"number\", or \"boolean\", got %q", r.Label, j, c.ValueType)
			}
		}
		// Verify each rule generates valid CEL.
		celExpr := rulesToCEL(r)
		if err := ValidateExpression(celExpr); err != nil {
			return fmt.Errorf("rule %q: %w", r.Label, err)
		}
	}
	return nil
}

// rulesToCEL generates a CEL boolean expression string from a single rule.
func rulesToCEL(r ConditionalRule) string {
	parts := make([]string, 0, len(r.Conditions))
	for _, c := range r.Conditions {
		ref := fmt.Sprintf(`ctx[%q][%q]`, c.NodeID, c.Field)
		var term string
		switch c.Operator {
		case "contains":
			term = fmt.Sprintf(`%s.contains(%s)`, ref, celLiteral(c.Value, "string"))
		default:
			term = fmt.Sprintf(`%s %s %s`, ref, c.Operator, celLiteral(c.Value, c.ValueType))
		}
		parts = append(parts, term)
	}
	joiner := " && "
	if r.Logic == "OR" {
		joiner = " || "
	}
	return strings.Join(parts, joiner)
}

// celLiteral formats a string value as a CEL literal based on value_type.
func celLiteral(value, valueType string) string {
	switch valueType {
	case "number":
		return value // stored as numeric string, used verbatim in CEL
	case "boolean":
		if value == "true" {
			return "true"
		}
		return "false"
	default: // "string"
		return fmt.Sprintf("%q", value)
	}
}

// Execute evaluates the conditional node. Detects the config format at runtime:
//   - Legacy format (config["expression"] is a non-empty string): evaluates the raw CEL,
//     returns {"result": bool}. Edge labels "true"/"false" continue to route correctly.
//   - New format (config["rules"] is present): evaluates each rule in order,
//     returns {"matched_rule": "<label>"} or {"matched_rule": "fallback"}.
func (h *Handler) Execute(_ context.Context, input node.NodeInput) (node.NodeOutput, error) {
	// Legacy path: raw expression.
	if expr, ok := input.Config["expression"].(string); ok && expr != "" {
		return h.executeLegacy(expr, input.UpstreamData)
	}

	// New path: structured rules.
	rawRules, ok := input.Config["rules"]
	if !ok {
		return node.NodeOutput{}, fmt.Errorf("conditional.branch: config must contain \"rules\" (new format) or \"expression\" (legacy)")
	}
	rules, err := parseRules(rawRules)
	if err != nil {
		return node.NodeOutput{}, fmt.Errorf("conditional.branch: parse rules: %w", err)
	}
	return h.executeRules(rules, input.UpstreamData)
}

func (h *Handler) executeLegacy(expr string, upstreamData map[string]any) (node.NodeOutput, error) {
	prg, err := h.getProgram(expr)
	if err != nil {
		return node.NodeOutput{}, fmt.Errorf("conditional.branch: %w", err)
	}
	out, _, err := prg.Eval(map[string]any{"ctx": upstreamData})
	if err != nil {
		return node.NodeOutput{}, fmt.Errorf("conditional.branch: evaluate: %w", err)
	}
	result, ok := out.Value().(bool)
	if !ok {
		return node.NodeOutput{}, fmt.Errorf("conditional.branch: expression must return bool, got %T", out.Value())
	}
	return node.NodeOutput{Data: map[string]any{"result": result}}, nil
}

func (h *Handler) executeRules(rules []ConditionalRule, upstreamData map[string]any) (node.NodeOutput, error) {
	ctx := map[string]any{"ctx": upstreamData}
	for _, r := range rules {
		celExpr := rulesToCEL(r)
		prg, err := h.getProgram(celExpr)
		if err != nil {
			return node.NodeOutput{}, fmt.Errorf("conditional.branch: rule %q: %w", r.Label, err)
		}
		out, _, err := prg.Eval(ctx)
		if err != nil {
			return node.NodeOutput{}, fmt.Errorf("conditional.branch: rule %q: evaluate: %w", r.Label, err)
		}
		matched, ok := out.Value().(bool)
		if !ok {
			return node.NodeOutput{}, fmt.Errorf("conditional.branch: rule %q: expression must return bool, got %T", r.Label, out.Value())
		}
		if matched {
			return node.NodeOutput{Data: map[string]any{"matched_rule": r.Label}}, nil
		}
	}
	return node.NodeOutput{Data: map[string]any{"matched_rule": "fallback"}}, nil
}

// parseRules unmarshals the rules field from config.
func parseRules(raw any) ([]ConditionalRule, error) {
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var rules []ConditionalRule
	if err := json.Unmarshal(b, &rules); err != nil {
		return nil, err
	}
	return rules, nil
}

// getProgram returns a cached cel.Program for expr, compiling it on first use.
func (h *Handler) getProgram(expr string) (cel.Program, error) {
	if v, ok := h.programs.Load(expr); ok {
		return v.(cel.Program), nil //nolint:forcetypeassert
	}
	env, err := sharedEnv()
	if err != nil {
		return nil, fmt.Errorf("create CEL env: %w", err)
	}
	ast, iss := env.Compile(expr)
	if iss.Err() != nil {
		return nil, fmt.Errorf("compile: %w", iss.Err())
	}
	prg, err := env.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("build program: %w", err)
	}
	h.programs.Store(expr, prg)
	return prg, nil
}

// sharedEnv is the package-level singleton CEL environment.
var (
	envOnce sync.Once
	envVal  *cel.Env
	envErr  error
)

func sharedEnv() (*cel.Env, error) {
	envOnce.Do(func() {
		envVal, envErr = cel.NewEnv(
			cel.Variable("ctx", cel.MapType(cel.StringType, cel.DynType)),
		)
	})
	return envVal, envErr
}

// newEnv creates a fresh CEL environment for ValidateExpression (save-time path).
func newEnv() (*cel.Env, error) {
	return cel.NewEnv(
		cel.Variable("ctx", cel.MapType(cel.StringType, cel.DynType)),
	)
}
