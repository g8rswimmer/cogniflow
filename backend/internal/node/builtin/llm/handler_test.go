package llm

import (
	"context"
	"errors"
	"testing"

	"github.com/g8rswimmer/cogniflow/internal/aiprovider"
	"github.com/g8rswimmer/cogniflow/internal/node"
)

// mockLLMClient is an in-process stub for aiprovider.LLMClient.
type mockLLMClient struct {
	resp    aiprovider.LLMResponse
	err     error
	lastReq aiprovider.LLMRequest
}

func (m *mockLLMClient) Complete(_ context.Context, req aiprovider.LLMRequest) (aiprovider.LLMResponse, error) {
	m.lastReq = req
	return m.resp, m.err
}

func TestLLMHandler_Meta_OpenAI(t *testing.T) {
	h := NewOpenAI(&mockLLMClient{})
	meta := h.Meta()
	if meta.TypeID != "llm.openai" {
		t.Errorf("want llm.openai, got %s", meta.TypeID)
	}
	if meta.Category != "ai" {
		t.Errorf("want category ai, got %s", meta.Category)
	}
}

func TestLLMHandler_Meta_Anthropic(t *testing.T) {
	h := NewAnthropic(&mockLLMClient{})
	meta := h.Meta()
	if meta.TypeID != "llm.anthropic" {
		t.Errorf("want llm.anthropic, got %s", meta.TypeID)
	}
}

func TestLLMHandler_Execute_Success(t *testing.T) {
	client := &mockLLMClient{
		resp: aiprovider.LLMResponse{
			Completion:       "Hello there!",
			PromptTokens:     10,
			CompletionTokens: 3,
		},
	}
	h := NewOpenAI(client)

	out, err := h.Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"api_key": "sk-test",
			"model":   "gpt-4o",
			"prompt":  "Say hello.",
		},
		UpstreamData: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Data["completion"] != "Hello there!" {
		t.Errorf("unexpected completion: %v", out.Data["completion"])
	}
	if out.Data["prompt_tokens"] != 10 {
		t.Errorf("unexpected prompt_tokens: %v", out.Data["prompt_tokens"])
	}
}

func TestLLMHandler_Execute_MissingPrompt(t *testing.T) {
	h := NewOpenAI(&mockLLMClient{})
	_, err := h.Execute(context.Background(), node.NodeInput{
		Config:       map[string]any{},
		UpstreamData: map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error for missing prompt")
	}
}

func TestLLMHandler_Execute_TemplateSubstitution(t *testing.T) {
	client := &mockLLMClient{
		resp: aiprovider.LLMResponse{Completion: "ok", PromptTokens: 1, CompletionTokens: 1},
	}
	h := NewOpenAI(client)

	_, err := h.Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"api_key": "k",
			"prompt":  "User ID is {{.n1.user_id}}",
		},
		UpstreamData: map[string]any{
			"n1": map[string]any{"user_id": "42"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.lastReq.Prompt != "User ID is 42" {
		t.Errorf("template not expanded: got %q", client.lastReq.Prompt)
	}
}

func TestLLMHandler_Execute_InitialDataTemplate(t *testing.T) {
	client := &mockLLMClient{
		resp: aiprovider.LLMResponse{Completion: "ok", PromptTokens: 1, CompletionTokens: 1},
	}
	h := NewOpenAI(client)

	_, err := h.Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"api_key": "k",
			"prompt":  "Check account for user {{._initial.customer_id}}",
		},
		UpstreamData: map[string]any{
			"_initial": map[string]any{"customer_id": "999"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.lastReq.Prompt != "Check account for user 999" {
		t.Errorf("initial data template not expanded: got %q", client.lastReq.Prompt)
	}
}

func TestLLMHandler_Execute_ClientError(t *testing.T) {
	h := NewOpenAI(&mockLLMClient{err: errors.New("rate limit")})

	_, err := h.Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"api_key": "k",
			"prompt":  "hello",
		},
		UpstreamData: map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error from client")
	}
}

func TestLLMHandler_Execute_InvalidPromptTemplate(t *testing.T) {
	h := NewOpenAI(&mockLLMClient{})
	_, err := h.Execute(context.Background(), node.NodeInput{
		Config:       map[string]any{"api_key": "k", "prompt": "{{.broken"},
		UpstreamData: map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error for invalid template")
	}
}

func TestLLMHandler_Execute_InvalidSystemMsgTemplate(t *testing.T) {
	h := NewOpenAI(&mockLLMClient{})
	_, err := h.Execute(context.Background(), node.NodeInput{
		Config:       map[string]any{"api_key": "k", "prompt": "hi", "system_msg": "{{.broken"},
		UpstreamData: map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error for invalid system_msg template")
	}
}

func TestLLMHandler_Execute_MaxTokensAndTemperature(t *testing.T) {
	client := &mockLLMClient{
		resp: aiprovider.LLMResponse{Completion: "ok", PromptTokens: 1, CompletionTokens: 1},
	}
	h := NewOpenAI(client)
	_, err := h.Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"api_key":     "k",
			"prompt":      "hi",
			"max_tokens":  float64(512),
			"temperature": float64(0.5),
		},
		UpstreamData: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.lastReq.MaxTokens != 512 {
		t.Errorf("want max_tokens=512, got %d", client.lastReq.MaxTokens)
	}
	if client.lastReq.Temperature == nil || *client.lastReq.Temperature != 0.5 {
		t.Errorf("want temperature=0.5, got %v", client.lastReq.Temperature)
	}
}

func TestLLMHandler_Execute_TemperatureZero(t *testing.T) {
	client := &mockLLMClient{
		resp: aiprovider.LLMResponse{Completion: "ok", PromptTokens: 1, CompletionTokens: 1},
	}
	h := NewOpenAI(client)
	_, err := h.Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"api_key":     "k",
			"prompt":      "hi",
			"temperature": float64(0),
		},
		UpstreamData: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Explicitly-set temperature=0 must be forwarded, not replaced with a default.
	if client.lastReq.Temperature == nil || *client.lastReq.Temperature != 0.0 {
		t.Errorf("want temperature=0.0 (greedy), got %v", client.lastReq.Temperature)
	}
}

func TestLLMHandler_Execute_TemperatureNotSet(t *testing.T) {
	client := &mockLLMClient{
		resp: aiprovider.LLMResponse{Completion: "ok", PromptTokens: 1, CompletionTokens: 1},
	}
	h := NewOpenAI(client)
	_, err := h.Execute(context.Background(), node.NodeInput{
		Config:       map[string]any{"api_key": "k", "prompt": "hi"},
		UpstreamData: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Unconfigured temperature must be nil so the provider uses its own default.
	if client.lastReq.Temperature != nil {
		t.Errorf("want nil temperature for unconfigured field, got %v", *client.lastReq.Temperature)
	}
}

func TestLLMHandler_Execute_SystemMsgTemplate(t *testing.T) {
	client := &mockLLMClient{
		resp: aiprovider.LLMResponse{Completion: "ok", PromptTokens: 1, CompletionTokens: 1},
	}
	h := NewAnthropic(client)

	_, err := h.Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"api_key":    "k",
			"prompt":     "hi",
			"system_msg": "Context: {{.n1.summary}}",
		},
		UpstreamData: map[string]any{
			"n1": map[string]any{"summary": "important context"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.lastReq.SystemMsg != "Context: important context" {
		t.Errorf("system_msg template not expanded: got %q", client.lastReq.SystemMsg)
	}
}
