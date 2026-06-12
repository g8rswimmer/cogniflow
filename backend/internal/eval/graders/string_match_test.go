package graders

import (
	"testing"

	"github.com/g8rswimmer/cogniflow/internal/store"
)

func def(graderType string, config map[string]any) store.GraderDef {
	return store.GraderDef{ID: "g1", Name: "test", Type: graderType, Config: config}
}

func TestStringMatch_Exact_Pass(t *testing.T) {
	g, err := NewStringMatch(def("string_match", map[string]any{
		"field_path": "result", "match_type": "exact", "expected_value": "hello",
	}))
	if err != nil {
		t.Fatal(err)
	}
	r := g.Grade(map[string]any{"result": "hello"})
	if r.Verdict != store.VerdictPass {
		t.Errorf("want pass, got %s: %s", r.Verdict, r.Explanation)
	}
}

func TestStringMatch_Exact_Fail(t *testing.T) {
	g, _ := NewStringMatch(def("string_match", map[string]any{
		"field_path": "result", "match_type": "exact", "expected_value": "hello",
	}))
	r := g.Grade(map[string]any{"result": "world"})
	if r.Verdict != store.VerdictFail {
		t.Errorf("want fail, got %s", r.Verdict)
	}
}

func TestStringMatch_Contains_Pass(t *testing.T) {
	g, _ := NewStringMatch(def("string_match", map[string]any{
		"field_path": "text", "match_type": "contains", "expected_value": "world",
	}))
	r := g.Grade(map[string]any{"text": "hello world"})
	if r.Verdict != store.VerdictPass {
		t.Errorf("want pass, got %s", r.Verdict)
	}
}

func TestStringMatch_Regex_Pass(t *testing.T) {
	g, err := NewStringMatch(def("string_match", map[string]any{
		"field_path": "msg", "match_type": "regex", "expected_value": `\d{3}`,
	}))
	if err != nil {
		t.Fatal(err)
	}
	r := g.Grade(map[string]any{"msg": "code 404"})
	if r.Verdict != store.VerdictPass {
		t.Errorf("want pass, got %s", r.Verdict)
	}
}

func TestStringMatch_Regex_Fail(t *testing.T) {
	g, _ := NewStringMatch(def("string_match", map[string]any{
		"field_path": "msg", "match_type": "regex", "expected_value": `^\d+$`,
	}))
	r := g.Grade(map[string]any{"msg": "not a number"})
	if r.Verdict != store.VerdictFail {
		t.Errorf("want fail, got %s", r.Verdict)
	}
}

func TestStringMatch_InvalidRegex(t *testing.T) {
	_, err := NewStringMatch(def("string_match", map[string]any{
		"field_path": "x", "match_type": "regex", "expected_value": "(broken",
	}))
	if err == nil {
		t.Error("expected error for invalid regex")
	}
}

func TestStringMatch_FieldNotFound(t *testing.T) {
	g, _ := NewStringMatch(def("string_match", map[string]any{
		"field_path": "missing", "match_type": "exact", "expected_value": "x",
	}))
	r := g.Grade(map[string]any{"other": "value"})
	if r.Verdict != store.VerdictError {
		t.Errorf("want error verdict, got %s", r.Verdict)
	}
}

func TestStringMatch_NestedFieldPath(t *testing.T) {
	g, _ := NewStringMatch(def("string_match", map[string]any{
		"field_path": "n1.completion", "match_type": "contains", "expected_value": "Hello",
	}))
	r := g.Grade(map[string]any{"n1": map[string]any{"completion": "Hello World"}})
	if r.Verdict != store.VerdictPass {
		t.Errorf("want pass for nested path, got %s", r.Verdict)
	}
}

func TestStringMatch_NonStringCoercion(t *testing.T) {
	g, _ := NewStringMatch(def("string_match", map[string]any{
		"field_path": "count", "match_type": "exact", "expected_value": "42",
	}))
	r := g.Grade(map[string]any{"count": float64(42)})
	if r.Verdict != store.VerdictPass {
		t.Errorf("want pass after coercion, got %s (%s)", r.Verdict, r.Explanation)
	}
}
