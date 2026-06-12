package graders

import (
	"context"
	"testing"

	"github.com/g8rswimmer/cogniflow/internal/store"
)

func schemaDef(fieldPath string, schema map[string]any) store.GraderDef {
	cfg := map[string]any{"schema": schema}
	if fieldPath != "" {
		cfg["field_path"] = fieldPath
	}
	return store.GraderDef{ID: "g1", Name: "test", Type: "json_schema", Config: cfg}
}

func TestJSONSchema_Pass_FullOutput(t *testing.T) {
	g, err := NewJSONSchema(schemaDef("", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"completion": map[string]any{"type": "string"},
		},
		"required": []any{"completion"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	r := g.Grade(context.Background(),map[string]any{"completion": "hello"})
	if r.Verdict != store.VerdictPass {
		t.Errorf("want pass, got %s: %s", r.Verdict, r.Explanation)
	}
}

func TestJSONSchema_Fail_MissingRequired(t *testing.T) {
	g, err := NewJSONSchema(schemaDef("", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"completion": map[string]any{"type": "string"},
		},
		"required": []any{"completion"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	r := g.Grade(context.Background(),map[string]any{"other": "value"})
	if r.Verdict != store.VerdictFail {
		t.Errorf("want fail, got %s", r.Verdict)
	}
	if r.Explanation == "" {
		t.Error("expected non-empty explanation on failure")
	}
}

func TestJSONSchema_Pass_FieldPath(t *testing.T) {
	g, err := NewJSONSchema(schemaDef("result", map[string]any{
		"type": "string",
	}))
	if err != nil {
		t.Fatal(err)
	}
	r := g.Grade(context.Background(),map[string]any{"result": "hello"})
	if r.Verdict != store.VerdictPass {
		t.Errorf("want pass, got %s: %s", r.Verdict, r.Explanation)
	}
}

func TestJSONSchema_Error_FieldNotFound(t *testing.T) {
	g, err := NewJSONSchema(schemaDef("missing_field", map[string]any{"type": "string"}))
	if err != nil {
		t.Fatal(err)
	}
	r := g.Grade(context.Background(),map[string]any{"other": "value"})
	if r.Verdict != store.VerdictError {
		t.Errorf("want error, got %s", r.Verdict)
	}
}

func TestJSONSchema_NoSchema_Error(t *testing.T) {
	_, err := NewJSONSchema(store.GraderDef{ID: "g1", Name: "test", Type: "json_schema", Config: map[string]any{}})
	if err == nil {
		t.Error("expected error when schema missing")
	}
}
