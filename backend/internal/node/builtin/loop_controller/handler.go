// Package loop_controller provides the loop.controller built-in node for cogniflow.
// The loop.controller node drives iterative execution of a loop body sub-graph.
// It is managed directly by the execution engine — this handler only decides
// whether to continue or exit the loop on each invocation.
package loop_controller

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
  "required": ["max_iterations"],
  "properties": {
    "max_iterations": {
      "type": "integer",
      "title": "Max Iterations",
      "description": "Hard cap on loop iterations. The loop exits gracefully when this limit is reached.",
      "minimum": 1,
      "maximum": 100,
      "default": 10
    },
    "exit_condition": {
      "type": "string",
      "title": "Exit Condition (CEL)",
      "description": "Optional CEL expression evaluated against upstream data each iteration. When true the loop exits. Omit to run exactly max_iterations times. Example: ctx[\"body_node\"][\"done\"] == true"
    }
  }
}`)

var outputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "action":      { "type": "string",  "description": "\"loop_body\" to continue, \"exit\" to stop" },
    "iteration":   { "type": "integer", "description": "Current iteration number (0-based)" },
    "exit_reason": { "type": "string",  "description": "Populated when action is \"exit\": \"condition\" or \"max_iterations\"" }
  }
}`)

// Handler implements the loop.controller node type.
// CEL programs are cached per expression so compilation happens only once.
type Handler struct {
	programs sync.Map // string → cel.Program
}

// New returns a Handler for the "loop.controller" node type.
func New() *Handler { return &Handler{} }

func (h *Handler) Meta() node.NodeMeta {
	return node.NodeMeta{
		TypeID:       "loop.controller",
		DisplayName:  "Loop Controller",
		Category:     "control",
		Description:  "Drives a loop body sub-graph for up to max_iterations times. An optional CEL exit condition can stop the loop early. Connect this node's \"loop_body\" edge to the first node in the loop body, and draw a loop-back edge from the last body node back to this controller. Connect the \"exit\" edge to the first node after the loop.",
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
	}
}

// Execute decides whether the loop should continue or exit.
// The engine injects "_loop_state" into UpstreamData before each call so the
// handler knows the current iteration number without needing external state.
func (h *Handler) Execute(_ context.Context, input node.NodeInput) (node.NodeOutput, error) {
	maxIter, err := maxIterations(input.Config)
	if err != nil {
		return node.NodeOutput{}, err
	}

	iteration := currentIteration(input.UpstreamData)

	// Hard cap: exit gracefully when limit reached.
	if iteration >= maxIter {
		return node.NodeOutput{Data: map[string]any{
			"action":      "exit",
			"iteration":   iteration,
			"exit_reason": "max_iterations",
		}}, nil
	}

	// Optional CEL exit condition.
	if expr, ok := input.Config["exit_condition"].(string); ok && expr != "" {
		shouldExit, err := h.evalExitCondition(expr, input.UpstreamData)
		if err != nil {
			return node.NodeOutput{}, fmt.Errorf("loop.controller: exit_condition: %w", err)
		}
		if shouldExit {
			return node.NodeOutput{Data: map[string]any{
				"action":      "exit",
				"iteration":   iteration,
				"exit_reason": "condition",
			}}, nil
		}
	}

	return node.NodeOutput{Data: map[string]any{
		"action":    "loop_body",
		"iteration": iteration,
	}}, nil
}

// ValidateExitCondition compiles the CEL expression and verifies it returns bool.
// Called at workflow save time.
func ValidateExitCondition(expr string) error {
	env, err := newCELEnv()
	if err != nil {
		return fmt.Errorf("loop.controller: create CEL env: %w", err)
	}
	ast, iss := env.Compile(expr)
	if iss.Err() != nil {
		return fmt.Errorf("loop.controller: invalid exit_condition: %w", iss.Err())
	}
	outName := ast.OutputType().TypeName()
	if outName != "bool" && outName != "dyn" {
		return fmt.Errorf("loop.controller: exit_condition must return bool, got %s", outName)
	}
	return nil
}

func (h *Handler) evalExitCondition(expr string, upstreamData map[string]any) (bool, error) {
	prg, err := h.getProgram(expr)
	if err != nil {
		return false, err
	}
	out, _, err := prg.Eval(map[string]any{"ctx": upstreamData})
	if err != nil {
		return false, fmt.Errorf("evaluate: %w", err)
	}
	result, ok := out.Value().(bool)
	if !ok {
		return false, fmt.Errorf("expression must return bool, got %T", out.Value())
	}
	return result, nil
}

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

// maxIterations reads the required max_iterations field from config.
func maxIterations(config map[string]any) (int, error) {
	v, ok := config["max_iterations"]
	if !ok {
		return 0, fmt.Errorf("loop.controller: max_iterations is required")
	}
	switch n := v.(type) {
	case float64:
		return int(n), nil
	case int:
		return n, nil
	case int64:
		return int(n), nil
	}
	return 0, fmt.Errorf("loop.controller: max_iterations must be a number, got %T", v)
}

// currentIteration reads the current loop iteration from the "_loop_state" key
// injected into UpstreamData by the engine before each controller dispatch.
func currentIteration(upstreamData map[string]any) int {
	state, ok := upstreamData["_loop_state"].(map[string]any)
	if !ok {
		return 0
	}
	switch n := state["iteration"].(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	}
	return 0
}

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

func newCELEnv() (*cel.Env, error) {
	return cel.NewEnv(
		cel.Variable("ctx", cel.MapType(cel.StringType, cel.DynType)),
	)
}
