package conditional

import (
	"context"
	"testing"

	"github.com/g8rswimmer/cogniflow/internal/node"
)

func TestHandler_Meta(t *testing.T) {
	h := New()
	meta := h.Meta()
	if meta.TypeID != "conditional.branch" {
		t.Errorf("want conditional.branch, got %s", meta.TypeID)
	}
	if meta.Category != "control" {
		t.Errorf("want control, got %s", meta.Category)
	}
}

// ---- ValidateExpression tests ------------------------------------------------

func TestValidateExpression_BoolLiteral(t *testing.T) {
	if err := ValidateExpression("true"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateExpression_Comparison(t *testing.T) {
	if err := ValidateExpression(`ctx["n1"]["status_code"] == 200`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateExpression_LogicalAnd(t *testing.T) {
	if err := ValidateExpression(`ctx["n1"]["ok"] == true && ctx["n2"]["score"] > 0`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateExpression_NonBoolReturnsError(t *testing.T) {
	err := ValidateExpression("1 + 1")
	if err == nil {
		t.Fatal("expected error for non-bool expression, got nil")
	}
}

func TestValidateExpression_SyntaxError(t *testing.T) {
	err := ValidateExpression("{{broken")
	if err == nil {
		t.Fatal("expected error for syntax error, got nil")
	}
}

func TestValidateExpression_StringLiteralReturnsError(t *testing.T) {
	err := ValidateExpression(`"hello"`)
	if err == nil {
		t.Fatal("expected error for string expression, got nil")
	}
}

// ---- Execute tests -----------------------------------------------------------

func TestExecute_TrueResult(t *testing.T) {
	h := New()
	out, err := h.Execute(context.Background(), node.NodeInput{
		Config: map[string]any{"expression": "true"},
		UpstreamData: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Data["result"] != true {
		t.Errorf("want true, got %v", out.Data["result"])
	}
}

func TestExecute_FalseResult(t *testing.T) {
	h := New()
	out, err := h.Execute(context.Background(), node.NodeInput{
		Config: map[string]any{"expression": "false"},
		UpstreamData: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Data["result"] != false {
		t.Errorf("want false, got %v", out.Data["result"])
	}
}

func TestExecute_StatusCodeComparison(t *testing.T) {
	h := New()
	out, err := h.Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"expression": `ctx["n1"]["status_code"] == 200`,
		},
		UpstreamData: map[string]any{
			"n1": map[string]any{"status_code": 200},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Data["result"] != true {
		t.Errorf("want true, got %v", out.Data["result"])
	}
}

func TestExecute_StatusCodeComparisonFalse(t *testing.T) {
	h := New()
	out, err := h.Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"expression": `ctx["n1"]["status_code"] == 200`,
		},
		UpstreamData: map[string]any{
			"n1": map[string]any{"status_code": 404},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Data["result"] != false {
		t.Errorf("want false, got %v", out.Data["result"])
	}
}

func TestExecute_LogicalExpression(t *testing.T) {
	h := New()
	out, err := h.Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"expression": `ctx["n1"]["score"] > 50 && ctx["n1"]["active"] == true`,
		},
		UpstreamData: map[string]any{
			"n1": map[string]any{"score": 75, "active": true},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Data["result"] != true {
		t.Errorf("want true, got %v", out.Data["result"])
	}
}

func TestExecute_MissingExpression_ReturnsError(t *testing.T) {
	h := New()
	_, err := h.Execute(context.Background(), node.NodeInput{
		Config:       map[string]any{},
		UpstreamData: map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error for missing expression")
	}
}

func TestExecute_InvalidExpression_ReturnsError(t *testing.T) {
	h := New()
	_, err := h.Execute(context.Background(), node.NodeInput{
		Config:       map[string]any{"expression": "{{broken"},
		UpstreamData: map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error for invalid expression")
	}
}

func TestExecute_NonBoolExpression_ReturnsError(t *testing.T) {
	h := New()
	_, err := h.Execute(context.Background(), node.NodeInput{
		Config:       map[string]any{"expression": "1 + 1"},
		UpstreamData: map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error for non-bool expression result")
	}
}

func TestExecute_InitialDataAccess(t *testing.T) {
	h := New()
	out, err := h.Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"expression": `ctx["_initial"]["flag"] == true`,
		},
		UpstreamData: map[string]any{
			"_initial": map[string]any{"flag": true},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Data["result"] != true {
		t.Errorf("want true, got %v", out.Data["result"])
	}
}
