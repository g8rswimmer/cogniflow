package loop_controller

import (
	"context"
	"testing"

	"github.com/g8rswimmer/cogniflow/internal/node"
)

func makeInput(config map[string]any, upstream map[string]any) node.NodeInput {
	return node.NodeInput{Config: config, UpstreamData: upstream}
}

func cfg(maxIter int) map[string]any {
	return map[string]any{"max_iterations": float64(maxIter)}
}

func cfgWithCondition(maxIter int, expr string) map[string]any {
	return map[string]any{"max_iterations": float64(maxIter), "exit_condition": expr}
}

func loopState(iteration int) map[string]any {
	return map[string]any{"_loop_state": map[string]any{"iteration": iteration}}
}

// TestExecute_FirstIteration verifies that with no loop state, iteration 0 returns loop_body.
func TestExecute_FirstIteration(t *testing.T) {
	h := New()
	out, err := h.Execute(context.Background(), makeInput(cfg(5), map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Data["action"] != "loop_body" {
		t.Errorf("expected action=loop_body, got %v", out.Data["action"])
	}
	if out.Data["iteration"] != 0 {
		t.Errorf("expected iteration=0, got %v", out.Data["iteration"])
	}
}

// TestExecute_ExitAtMaxIterations verifies that when iteration == max_iterations, action=exit.
func TestExecute_ExitAtMaxIterations(t *testing.T) {
	h := New()
	out, err := h.Execute(context.Background(), makeInput(cfg(3), loopState(3)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Data["action"] != "exit" {
		t.Errorf("expected action=exit, got %v", out.Data["action"])
	}
	if out.Data["exit_reason"] != "max_iterations" {
		t.Errorf("expected exit_reason=max_iterations, got %v", out.Data["exit_reason"])
	}
}

// TestExecute_ContinuesBelowMax verifies that iteration < max_iterations returns loop_body.
func TestExecute_ContinuesBelowMax(t *testing.T) {
	h := New()
	out, err := h.Execute(context.Background(), makeInput(cfg(10), loopState(7)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Data["action"] != "loop_body" {
		t.Errorf("expected action=loop_body, got %v", out.Data["action"])
	}
}

// TestExecute_CELExitConditionTrue verifies that a true exit_condition causes action=exit.
func TestExecute_CELExitConditionTrue(t *testing.T) {
	h := New()
	upstream := map[string]any{
		"body_node": map[string]any{"done": true},
	}
	input := makeInput(cfgWithCondition(10, `ctx["body_node"]["done"] == true`), upstream)
	out, err := h.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Data["action"] != "exit" {
		t.Errorf("expected action=exit when condition true, got %v", out.Data["action"])
	}
	if out.Data["exit_reason"] != "condition" {
		t.Errorf("expected exit_reason=condition, got %v", out.Data["exit_reason"])
	}
}

// TestExecute_CELExitConditionFalse verifies that a false exit_condition returns loop_body.
func TestExecute_CELExitConditionFalse(t *testing.T) {
	h := New()
	upstream := map[string]any{
		"body_node": map[string]any{"done": false},
	}
	input := makeInput(cfgWithCondition(10, `ctx["body_node"]["done"] == true`), upstream)
	out, err := h.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Data["action"] != "loop_body" {
		t.Errorf("expected action=loop_body when condition false, got %v", out.Data["action"])
	}
}

// TestExecute_MissingMaxIterations verifies that a missing max_iterations returns an error.
func TestExecute_MissingMaxIterations(t *testing.T) {
	h := New()
	_, err := h.Execute(context.Background(), makeInput(map[string]any{}, map[string]any{}))
	if err == nil {
		t.Fatal("expected error for missing max_iterations")
	}
}

// TestValidateExitCondition_Valid verifies that a valid bool CEL expression passes.
func TestValidateExitCondition_Valid(t *testing.T) {
	if err := ValidateExitCondition(`ctx["node"]["field"] == "value"`); err != nil {
		t.Errorf("expected valid expression to pass, got: %v", err)
	}
}

// TestValidateExitCondition_Invalid verifies that a malformed CEL expression is rejected.
func TestValidateExitCondition_Invalid(t *testing.T) {
	if err := ValidateExitCondition(`!!!not valid cel!!!`); err == nil {
		t.Error("expected error for invalid CEL expression")
	}
}

// TestMeta verifies the node's type ID and required schema fields are set.
func TestMeta(t *testing.T) {
	h := New()
	m := h.Meta()
	if m.TypeID != "loop.controller" {
		t.Errorf("unexpected TypeID: %s", m.TypeID)
	}
	if len(m.InputSchema) == 0 {
		t.Error("InputSchema must not be empty")
	}
	if len(m.OutputSchema) == 0 {
		t.Error("OutputSchema must not be empty")
	}
}
