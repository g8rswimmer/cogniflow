// Package conditional provides the conditional.branch built-in node for cogniflow.
package conditional

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/google/cel-go/cel"

	"github.com/g8rswimmer/cogniflow/internal/node"
)

var inputSchema = json.RawMessage(`{
  "type": "object",
  "required": ["expression"],
  "properties": {
    "expression": {
      "type": "string",
      "title": "CEL Expression",
      "description": "Boolean CEL expression. Reference upstream outputs via ctx[\"nodeID\"][\"field\"]. Example: ctx[\"n1\"][\"status_code\"] == 200"
    }
  }
}`)

var outputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "result": { "type": "boolean", "description": "Evaluated result; edges with branch_label \"true\" or \"false\" are filtered accordingly" }
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
		Description:  "Evaluates a CEL boolean expression and routes execution to the matching branch (true or false).",
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
	}
}

// ValidateExpression compiles the CEL expression and verifies it returns bool (or dyn).
// Call this at workflow save time to surface syntax and type errors early.
// Note: expressions whose return type cannot be resolved statically (e.g. map
// field lookups) compile as "dyn" and pass this check; they must ultimately
// evaluate to a bool at runtime or the node will return an error.
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

// Execute evaluates the configured CEL expression against upstream node outputs.
// The compiled program is cached on the Handler so repeated executions of the
// same workflow pay the CEL parse/type-check cost only once.
func (h *Handler) Execute(_ context.Context, input node.NodeInput) (node.NodeOutput, error) {
	expr, _ := input.Config["expression"].(string)
	if expr == "" {
		return node.NodeOutput{}, fmt.Errorf("conditional.branch: expression is required")
	}

	prg, err := h.getProgram(expr)
	if err != nil {
		return node.NodeOutput{}, fmt.Errorf("conditional.branch: %w", err)
	}

	out, _, err := prg.Eval(map[string]any{
		"ctx": input.UpstreamData,
	})
	if err != nil {
		return node.NodeOutput{}, fmt.Errorf("conditional.branch: evaluate: %w", err)
	}

	result, ok := out.Value().(bool)
	if !ok {
		return node.NodeOutput{}, fmt.Errorf("conditional.branch: expression must return bool, got %T — use a comparison (e.g. ctx[\"n1\"][\"field\"] == 200)", out.Value())
	}

	return node.NodeOutput{Data: map[string]any{"result": result}}, nil
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

// sharedEnv is the package-level singleton CEL environment. Built once so that
// environment construction cost is amortised across all conditional nodes.
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

// newEnv creates a fresh CEL environment. Used only by ValidateExpression,
// which is a save-time path and not performance-sensitive.
func newEnv() (*cel.Env, error) {
	return cel.NewEnv(
		cel.Variable("ctx", cel.MapType(cel.StringType, cel.DynType)),
	)
}
