package eval

import (
	"testing"

	"github.com/g8rswimmer/cogniflow/internal/store"
)

func TestBuildGrader_StringMatch(t *testing.T) {
	g, err := BuildGrader(store.GraderDef{
		ID: "g1", Type: "string_match",
		Config: map[string]any{"field_path": "x", "match_type": "exact", "expected_value": "ok"},
	})
	if err != nil {
		t.Fatalf("BuildGrader: %v", err)
	}
	r := g.Grade(map[string]any{"x": "ok"})
	if r.Verdict != store.VerdictPass {
		t.Errorf("want pass, got %s", r.Verdict)
	}
}

func TestBuildGrader_NumericThreshold(t *testing.T) {
	g, err := BuildGrader(store.GraderDef{
		ID: "g1", Type: "numeric_threshold",
		Config: map[string]any{"field_path": "n", "operator": "==", "threshold": float64(42)},
	})
	if err != nil {
		t.Fatalf("BuildGrader: %v", err)
	}
	r := g.Grade(map[string]any{"n": float64(42)})
	if r.Verdict != store.VerdictPass {
		t.Errorf("want pass, got %s", r.Verdict)
	}
}

func TestBuildGrader_JSONSchema(t *testing.T) {
	g, err := BuildGrader(store.GraderDef{
		ID: "g1", Type: "json_schema",
		Config: map[string]any{"schema": map[string]any{"type": "object"}},
	})
	if err != nil {
		t.Fatalf("BuildGrader: %v", err)
	}
	r := g.Grade(map[string]any{"anything": "goes"})
	if r.Verdict != store.VerdictPass {
		t.Errorf("want pass, got %s", r.Verdict)
	}
}

func TestBuildGrader_LLMJudge_NotAvailable(t *testing.T) {
	_, err := BuildGrader(store.GraderDef{ID: "g1", Type: "llm_judge", Config: map[string]any{}})
	if err == nil {
		t.Error("expected error for llm_judge (requires ME3)")
	}
}

func TestBuildGrader_Checklist_NotAvailable(t *testing.T) {
	_, err := BuildGrader(store.GraderDef{ID: "g1", Type: "checklist", Config: map[string]any{}})
	if err == nil {
		t.Error("expected error for checklist (requires ME3)")
	}
}

func TestBuildGrader_Unknown(t *testing.T) {
	_, err := BuildGrader(store.GraderDef{ID: "g1", Type: "bogus", Config: map[string]any{}})
	if err == nil {
		t.Error("expected error for unknown grader type")
	}
}
