package graders

import (
	"context"
	"errors"
	"testing"

	"github.com/g8rswimmer/cogniflow/internal/aiprovider"
	"github.com/g8rswimmer/cogniflow/internal/store"
)

// mockLLMClient satisfies aiprovider.LLMClient for testing. Shared by llm_judge and checklist tests.
type mockLLMClient struct {
	resp aiprovider.LLMResponse
	err  error
}

func (m *mockLLMClient) Complete(_ context.Context, _ aiprovider.LLMRequest) (aiprovider.LLMResponse, error) {
	return m.resp, m.err
}

func judgeGraderDef(rubric, fieldPath string) store.GraderDef {
	return store.GraderDef{
		ID:   "g1",
		Name: "Judge",
		Type: "llm_judge",
		Config: map[string]any{
			"provider":   "anthropic",
			"model":      "claude-haiku-4-5-20251001",
			"api_key":    "test-key",
			"rubric":     rubric,
			"field_path": fieldPath,
		},
	}
}

func TestLLMJudge_Pass(t *testing.T) {
	client := &mockLLMClient{
		resp: aiprovider.LLMResponse{Completion: `{"verdict":"pass","explanation":"Meets the rubric."}`},
	}
	g, err := NewLLMJudge(judgeGraderDef("Response is helpful", ""), client)
	if err != nil {
		t.Fatal(err)
	}
	r := g.Grade(context.Background(), map[string]any{"completion": "This is a helpful response."})
	if r.Verdict != store.VerdictPass {
		t.Errorf("want pass, got %s: %s", r.Verdict, r.Explanation)
	}
	if r.Explanation == "" {
		t.Error("expected non-empty explanation")
	}
}

func TestLLMJudge_Fail(t *testing.T) {
	client := &mockLLMClient{
		resp: aiprovider.LLMResponse{Completion: `{"verdict":"fail","explanation":"Does not address the question."}`},
	}
	g, _ := NewLLMJudge(judgeGraderDef("Response must address the question", ""), client)
	r := g.Grade(context.Background(), map[string]any{"completion": "Unrelated content."})
	if r.Verdict != store.VerdictFail {
		t.Errorf("want fail, got %s", r.Verdict)
	}
	if r.Explanation == "" {
		t.Error("expected non-empty explanation on fail")
	}
}

func TestLLMJudge_FieldPath(t *testing.T) {
	client := &mockLLMClient{
		resp: aiprovider.LLMResponse{Completion: `{"verdict":"pass","explanation":"ok"}`},
	}
	g, _ := NewLLMJudge(judgeGraderDef("Is helpful", "n1.completion"), client)
	r := g.Grade(context.Background(), map[string]any{"n1": map[string]any{"completion": "helpful answer"}})
	if r.Verdict != store.VerdictPass {
		t.Errorf("want pass, got %s: %s", r.Verdict, r.Explanation)
	}
	if r.ActualValue == nil {
		t.Error("expected ActualValue to be set")
	}
}

func TestLLMJudge_FieldNotFound(t *testing.T) {
	client := &mockLLMClient{}
	g, _ := NewLLMJudge(judgeGraderDef("rubric", "missing.field"), client)
	r := g.Grade(context.Background(), map[string]any{"other": "value"})
	if r.Verdict != store.VerdictError {
		t.Errorf("want error verdict, got %s", r.Verdict)
	}
}

func TestLLMJudge_LLMError(t *testing.T) {
	client := &mockLLMClient{err: errors.New("anthropic: 401 Unauthorized")}
	g, _ := NewLLMJudge(judgeGraderDef("rubric", ""), client)
	r := g.Grade(context.Background(), map[string]any{"x": 1})
	if r.Verdict != store.VerdictError {
		t.Errorf("want error verdict, got %s", r.Verdict)
	}
	if r.Explanation == "" {
		t.Error("expected explanation with LLM error message")
	}
}

func TestLLMJudge_ParseError(t *testing.T) {
	client := &mockLLMClient{
		resp: aiprovider.LLMResponse{Completion: "I cannot determine this."},
	}
	g, _ := NewLLMJudge(judgeGraderDef("rubric", ""), client)
	r := g.Grade(context.Background(), map[string]any{"x": 1})
	if r.Verdict != store.VerdictError {
		t.Errorf("want error on parse failure, got %s", r.Verdict)
	}
}

func TestLLMJudge_UnknownVerdict(t *testing.T) {
	client := &mockLLMClient{
		resp: aiprovider.LLMResponse{Completion: `{"verdict":"maybe","explanation":"unsure"}`},
	}
	g, _ := NewLLMJudge(judgeGraderDef("rubric", ""), client)
	r := g.Grade(context.Background(), map[string]any{"x": 1})
	if r.Verdict != store.VerdictError {
		t.Errorf("want error for unknown verdict, got %s", r.Verdict)
	}
}

func TestLLMJudge_MissingRubric(t *testing.T) {
	_, err := NewLLMJudge(store.GraderDef{
		ID:   "g1",
		Type: "llm_judge",
		Config: map[string]any{
			"provider": "anthropic",
			"model":    "claude-haiku-4-5-20251001",
		},
	}, &mockLLMClient{})
	if err == nil {
		t.Error("expected error when rubric is missing")
	}
}

func TestLLMJudge_JSONPreambleTolerated(t *testing.T) {
	client := &mockLLMClient{
		resp: aiprovider.LLMResponse{
			Completion: `Here is my evaluation: {"verdict":"pass","explanation":"Good response."}`,
		},
	}
	g, _ := NewLLMJudge(judgeGraderDef("rubric", ""), client)
	r := g.Grade(context.Background(), map[string]any{"x": 1})
	if r.Verdict != store.VerdictPass {
		t.Errorf("want pass after preamble stripping, got %s: %s", r.Verdict, r.Explanation)
	}
}
