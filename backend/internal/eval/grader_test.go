package eval

import (
	"context"
	"fmt"
	"testing"

	"github.com/g8rswimmer/cogniflow/internal/aiprovider"
	"github.com/g8rswimmer/cogniflow/internal/store"
)

// stubLLMClient satisfies aiprovider.LLMClient for BuildGrader dispatch tests.
type stubLLMClient struct {
	completion string
	err        error
}

func (s *stubLLMClient) Complete(_ context.Context, _ aiprovider.LLMRequest) (aiprovider.LLMResponse, error) {
	return aiprovider.LLMResponse{Completion: s.completion}, s.err
}

// okFactory always returns the provided client regardless of provider.
func okFactory(client aiprovider.LLMClient) LLMFactory {
	return func(_ string) (aiprovider.LLMClient, error) {
		return client, nil
	}
}

func TestBuildGrader_StringMatch(t *testing.T) {
	g, err := BuildGrader(store.GraderDef{
		ID: "g1", Type: "string_match",
		Config: map[string]any{"field_path": "x", "match_type": "exact", "expected_value": "ok"},
	}, nil, nil)
	if err != nil {
		t.Fatalf("BuildGrader: %v", err)
	}
	r := g.Grade(context.Background(), map[string]any{"x": "ok"})
	if r.Verdict != store.VerdictPass {
		t.Errorf("want pass, got %s", r.Verdict)
	}
}

func TestBuildGrader_NumericThreshold(t *testing.T) {
	g, err := BuildGrader(store.GraderDef{
		ID: "g1", Type: "numeric_threshold",
		Config: map[string]any{"field_path": "n", "operator": "==", "threshold": float64(42)},
	}, nil, nil)
	if err != nil {
		t.Fatalf("BuildGrader: %v", err)
	}
	r := g.Grade(context.Background(), map[string]any{"n": float64(42)})
	if r.Verdict != store.VerdictPass {
		t.Errorf("want pass, got %s", r.Verdict)
	}
}

func TestBuildGrader_JSONSchema(t *testing.T) {
	g, err := BuildGrader(store.GraderDef{
		ID: "g1", Type: "json_schema",
		Config: map[string]any{"schema": map[string]any{"type": "object"}},
	}, nil, nil)
	if err != nil {
		t.Fatalf("BuildGrader: %v", err)
	}
	r := g.Grade(context.Background(), map[string]any{"anything": "goes"})
	if r.Verdict != store.VerdictPass {
		t.Errorf("want pass, got %s", r.Verdict)
	}
}

func TestBuildGrader_LLMJudge_NilFactory(t *testing.T) {
	_, err := BuildGrader(store.GraderDef{ID: "g1", Type: "llm_judge", Config: map[string]any{"rubric": "test"}}, nil, nil)
	if err == nil {
		t.Error("expected error for llm_judge with nil factory")
	}
}

func TestBuildGrader_Checklist_NilFactory(t *testing.T) {
	_, err := BuildGrader(store.GraderDef{ID: "g1", Type: "checklist", Config: map[string]any{"criteria": []any{"c1"}}}, nil, nil)
	if err == nil {
		t.Error("expected error for checklist with nil factory")
	}
}

func TestBuildGrader_LLMJudge_WithFactory(t *testing.T) {
	client := &stubLLMClient{completion: `{"verdict":"pass","explanation":"ok"}`}
	g, err := BuildGrader(store.GraderDef{
		ID:   "g1",
		Type: "llm_judge",
		Config: map[string]any{
			"provider": "anthropic",
			"model":    "claude-haiku-4-5-20251001",
			"api_key":  "key",
			"rubric":   "Is it good?",
		},
	}, okFactory(client), nil)
	if err != nil {
		t.Fatalf("BuildGrader: %v", err)
	}
	r := g.Grade(context.Background(), map[string]any{"completion": "yes"})
	if r.Verdict != store.VerdictPass {
		t.Errorf("want pass, got %s: %s", r.Verdict, r.Explanation)
	}
}

func TestBuildGrader_Checklist_WithFactory(t *testing.T) {
	client := &stubLLMClient{completion: `[{"criterion":"Is good","met":true,"explanation":"yes"}]`}
	g, err := BuildGrader(store.GraderDef{
		ID:   "g1",
		Type: "checklist",
		Config: map[string]any{
			"provider": "anthropic",
			"model":    "claude-haiku-4-5-20251001",
			"api_key":  "key",
			"criteria": []any{"Is good"},
		},
	}, okFactory(client), nil)
	if err != nil {
		t.Fatalf("BuildGrader: %v", err)
	}
	r := g.Grade(context.Background(), map[string]any{"completion": "yes"})
	if r.Verdict != store.VerdictPass {
		t.Errorf("want pass, got %s: %s", r.Verdict, r.Explanation)
	}
}

func TestBuildGrader_LLMJudge_FactoryError(t *testing.T) {
	badFactory := LLMFactory(func(provider string) (aiprovider.LLMClient, error) {
		return nil, fmt.Errorf("unknown provider %q", provider)
	})
	_, err := BuildGrader(store.GraderDef{
		ID:   "g1",
		Type: "llm_judge",
		Config: map[string]any{
			"provider": "unknown_provider",
			"rubric":   "test",
		},
	}, badFactory, nil)
	if err == nil {
		t.Error("expected error when factory fails")
	}
}

func TestBuildGrader_Unknown(t *testing.T) {
	_, err := BuildGrader(store.GraderDef{ID: "g1", Type: "bogus", Config: map[string]any{}}, nil, nil)
	if err == nil {
		t.Error("expected error for unknown grader type")
	}
}

func TestBuildGrader_Unknown_NilRegistryFallsThrough(t *testing.T) {
	// nil registry should not panic; unknown type must still error.
	_, err := BuildGrader(store.GraderDef{ID: "g1", Type: "custom.grader", Config: map[string]any{}}, nil, nil)
	if err == nil {
		t.Error("expected error for unknown grader type with nil registry")
	}
}
