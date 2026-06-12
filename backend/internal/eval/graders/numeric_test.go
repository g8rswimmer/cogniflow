package graders

import (
	"testing"

	"github.com/g8rswimmer/cogniflow/internal/store"
)

func numericDef(op string, threshold float64) store.GraderDef {
	return store.GraderDef{ID: "g1", Name: "test", Type: "numeric_threshold", Config: map[string]any{
		"field_path": "value", "operator": op, "threshold": threshold,
	}}
}

func TestNumeric_Equal_Pass(t *testing.T) {
	g, err := NewNumericThreshold(numericDef("==", 200))
	if err != nil {
		t.Fatal(err)
	}
	r := g.Grade(map[string]any{"value": float64(200)})
	if r.Verdict != store.VerdictPass {
		t.Errorf("want pass, got %s: %s", r.Verdict, r.Explanation)
	}
}

func TestNumeric_Equal_Fail(t *testing.T) {
	g, _ := NewNumericThreshold(numericDef("==", 200))
	r := g.Grade(map[string]any{"value": float64(404)})
	if r.Verdict != store.VerdictFail {
		t.Errorf("want fail, got %s", r.Verdict)
	}
}

func TestNumeric_LessThan_Pass(t *testing.T) {
	g, _ := NewNumericThreshold(numericDef("<", 500))
	r := g.Grade(map[string]any{"value": float64(42)})
	if r.Verdict != store.VerdictPass {
		t.Errorf("want pass, got %s", r.Verdict)
	}
}

func TestNumeric_GreaterThanEqual_Pass(t *testing.T) {
	g, _ := NewNumericThreshold(numericDef(">=", 0.8))
	r := g.Grade(map[string]any{"value": float64(1.0)})
	if r.Verdict != store.VerdictPass {
		t.Errorf("want pass, got %s", r.Verdict)
	}
}

func TestNumeric_FieldNotFound(t *testing.T) {
	g, _ := NewNumericThreshold(numericDef("==", 1))
	r := g.Grade(map[string]any{"other": float64(1)})
	if r.Verdict != store.VerdictError {
		t.Errorf("want error, got %s", r.Verdict)
	}
}

func TestNumeric_NonNumericField(t *testing.T) {
	g, _ := NewNumericThreshold(numericDef("==", 1))
	r := g.Grade(map[string]any{"value": "not a number"})
	if r.Verdict != store.VerdictError {
		t.Errorf("want error, got %s", r.Verdict)
	}
}

func TestNumeric_InvalidOperator(t *testing.T) {
	_, err := NewNumericThreshold(store.GraderDef{ID: "g1", Name: "test", Type: "numeric_threshold", Config: map[string]any{
		"field_path": "x", "operator": "<>", "threshold": float64(0),
	}})
	if err == nil {
		t.Error("expected error for invalid operator")
	}
}

func TestNumeric_IntThreshold(t *testing.T) {
	// threshold provided as int (JSON unmarshalling can produce float64, but Go config map may have int)
	g, err := NewNumericThreshold(store.GraderDef{ID: "g1", Name: "test", Type: "numeric_threshold", Config: map[string]any{
		"field_path": "count", "operator": "==", "threshold": 100,
	}})
	if err != nil {
		t.Fatal(err)
	}
	r := g.Grade(map[string]any{"count": float64(100)})
	if r.Verdict != store.VerdictPass {
		t.Errorf("want pass, got %s", r.Verdict)
	}
}
