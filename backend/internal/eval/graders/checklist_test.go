package graders

import (
	"context"
	"errors"
	"testing"

	"github.com/g8rswimmer/cogniflow/internal/aiprovider"
	"github.com/g8rswimmer/cogniflow/internal/store"
)

func checklistDef(criteria []any, passThreshold float64) store.GraderDef {
	return store.GraderDef{
		ID:   "g1",
		Name: "Checklist",
		Type: "checklist",
		Config: map[string]any{
			"provider":       "anthropic",
			"model":          "claude-haiku-4-5-20251001",
			"api_key":        "test-key",
			"criteria":       criteria,
			"pass_threshold": passThreshold,
		},
	}
}

func TestChecklist_AllPass(t *testing.T) {
	client := &mockLLMClient{
		resp: aiprovider.LLMResponse{Completion: `[{"criterion":"Is professional","met":true,"explanation":"Formal tone"},{"criterion":"Is concise","met":true,"explanation":"Short and clear"}]`},
	}
	g, err := NewChecklist(checklistDef([]any{"Is professional", "Is concise"}, 1.0), client)
	if err != nil {
		t.Fatal(err)
	}
	r := g.Grade(context.Background(), map[string]any{"completion": "Dear customer, your issue has been resolved."})
	if r.Verdict != store.VerdictPass {
		t.Errorf("want pass, got %s: %s", r.Verdict, r.Explanation)
	}
	if r.Score == nil || *r.Score != 1.0 {
		t.Errorf("want score=1.0, got %v", r.Score)
	}
	if len(r.CriteriaResults) != 2 {
		t.Errorf("want 2 criteria results, got %d", len(r.CriteriaResults))
	}
}

func TestChecklist_PartialPass_AboveThreshold(t *testing.T) {
	client := &mockLLMClient{
		resp: aiprovider.LLMResponse{Completion: `[{"criterion":"c1","met":true,"explanation":"ok"},{"criterion":"c2","met":true,"explanation":"ok"},{"criterion":"c3","met":false,"explanation":"no"},{"criterion":"c4","met":true,"explanation":"ok"},{"criterion":"c5","met":false,"explanation":"no"}]`},
	}
	g, err := NewChecklist(checklistDef([]any{"c1", "c2", "c3", "c4", "c5"}, 0.6), client)
	if err != nil {
		t.Fatal(err)
	}
	r := g.Grade(context.Background(), map[string]any{"text": "content"})
	if r.Verdict != store.VerdictPass {
		t.Errorf("3/5=0.6 >= threshold 0.6 should pass, got %s", r.Verdict)
	}
	if r.Score == nil {
		t.Fatal("expected score to be set")
	}
	const want = 0.6
	if *r.Score != want {
		t.Errorf("want score=%.1f, got %.2f", want, *r.Score)
	}
}

func TestChecklist_PartialFail_BelowThreshold(t *testing.T) {
	client := &mockLLMClient{
		resp: aiprovider.LLMResponse{Completion: `[{"criterion":"c1","met":false,"explanation":"no"},{"criterion":"c2","met":false,"explanation":"no"},{"criterion":"c3","met":true,"explanation":"yes"}]`},
	}
	g, err := NewChecklist(checklistDef([]any{"c1", "c2", "c3"}, 0.8), client)
	if err != nil {
		t.Fatal(err)
	}
	r := g.Grade(context.Background(), map[string]any{"text": "content"})
	if r.Verdict != store.VerdictFail {
		t.Errorf("1/3≈0.33 < threshold 0.8 should fail, got %s", r.Verdict)
	}
}

func TestChecklist_LLMError(t *testing.T) {
	client := &mockLLMClient{err: errors.New("rate limit exceeded")}
	g, _ := NewChecklist(checklistDef([]any{"criterion"}, 1.0), client)
	r := g.Grade(context.Background(), map[string]any{"x": 1})
	if r.Verdict != store.VerdictError {
		t.Errorf("want error, got %s", r.Verdict)
	}
}

func TestChecklist_ParseError(t *testing.T) {
	client := &mockLLMClient{resp: aiprovider.LLMResponse{Completion: "I evaluated the criteria and they all seem fine."}}
	g, _ := NewChecklist(checklistDef([]any{"criterion"}, 1.0), client)
	r := g.Grade(context.Background(), map[string]any{"x": 1})
	if r.Verdict != store.VerdictError {
		t.Errorf("want error on parse failure, got %s", r.Verdict)
	}
}

func TestChecklist_FieldPath(t *testing.T) {
	client := &mockLLMClient{
		resp: aiprovider.LLMResponse{Completion: `[{"criterion":"Is string","met":true,"explanation":"yes"}]`},
	}
	g, _ := NewChecklist(store.GraderDef{
		ID:   "g1",
		Type: "checklist",
		Config: map[string]any{
			"provider":  "anthropic",
			"model":     "claude-haiku-4-5-20251001",
			"api_key":   "key",
			"criteria":  []any{"Is string"},
			"field_path": "n1.completion",
		},
	}, client)
	r := g.Grade(context.Background(), map[string]any{"n1": map[string]any{"completion": "hello"}})
	if r.Verdict != store.VerdictPass {
		t.Errorf("want pass, got %s: %s", r.Verdict, r.Explanation)
	}
}

func TestChecklist_FieldNotFound(t *testing.T) {
	client := &mockLLMClient{}
	g, _ := NewChecklist(store.GraderDef{
		ID:   "g1",
		Type: "checklist",
		Config: map[string]any{
			"criteria":   []any{"c1"},
			"field_path": "missing.field",
		},
	}, client)
	r := g.Grade(context.Background(), map[string]any{"other": "value"})
	if r.Verdict != store.VerdictError {
		t.Errorf("want error verdict, got %s", r.Verdict)
	}
}

func TestChecklist_NoCriteria(t *testing.T) {
	_, err := NewChecklist(store.GraderDef{
		ID:   "g1",
		Type: "checklist",
		Config: map[string]any{
			"provider": "anthropic",
			"model":    "claude-haiku-4-5-20251001",
		},
	}, &mockLLMClient{})
	if err == nil {
		t.Error("expected error when criteria is missing")
	}
}

func TestChecklist_EmptyCriteria(t *testing.T) {
	_, err := NewChecklist(checklistDef([]any{}, 1.0), &mockLLMClient{})
	if err == nil {
		t.Error("expected error for empty criteria array")
	}
}

func TestChecklist_JSONPreambleTolerated(t *testing.T) {
	client := &mockLLMClient{
		resp: aiprovider.LLMResponse{
			Completion: `Here are my results: [{"criterion":"Is helpful","met":true,"explanation":"Yes it is"}]`,
		},
	}
	g, _ := NewChecklist(checklistDef([]any{"Is helpful"}, 1.0), client)
	r := g.Grade(context.Background(), map[string]any{"x": 1})
	if r.Verdict != store.VerdictPass {
		t.Errorf("want pass after preamble stripping, got %s: %s", r.Verdict, r.Explanation)
	}
}
