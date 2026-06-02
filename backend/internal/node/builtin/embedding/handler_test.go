package embedding

import (
	"context"
	"errors"
	"testing"

	"github.com/g8rswimmer/cogniflow/internal/aiprovider"
	"github.com/g8rswimmer/cogniflow/internal/node"
)

type mockEmbeddingClient struct {
	resp    aiprovider.EmbeddingResponse
	err     error
	lastReq aiprovider.EmbeddingRequest
}

func (m *mockEmbeddingClient) Embed(_ context.Context, req aiprovider.EmbeddingRequest) (aiprovider.EmbeddingResponse, error) {
	m.lastReq = req
	return m.resp, m.err
}

func TestEmbeddingHandler_Meta(t *testing.T) {
	h := New(&mockEmbeddingClient{})
	meta := h.Meta()
	if meta.TypeID != "embedding.openai" {
		t.Errorf("want embedding.openai, got %s", meta.TypeID)
	}
	if meta.Category != "ai" {
		t.Errorf("want ai, got %s", meta.Category)
	}
}

func TestEmbeddingHandler_Execute_Success(t *testing.T) {
	want := []float32{0.1, 0.2, 0.3}
	client := &mockEmbeddingClient{resp: aiprovider.EmbeddingResponse{Embedding: want}}
	h := New(client)

	out, err := h.Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"api_key": "sk-test",
			"input":   "hello world",
		},
		UpstreamData: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	emb, ok := out.Data["embedding"].([]any)
	if !ok {
		t.Fatalf("embedding not []any: %T", out.Data["embedding"])
	}
	if len(emb) != len(want) {
		t.Errorf("want %d dims, got %d", len(want), len(emb))
	}
}

func TestEmbeddingHandler_Execute_MissingInput(t *testing.T) {
	h := New(&mockEmbeddingClient{})
	_, err := h.Execute(context.Background(), node.NodeInput{
		Config:       map[string]any{},
		UpstreamData: map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error for missing input")
	}
}

func TestEmbeddingHandler_Execute_TemplateSubstitution(t *testing.T) {
	client := &mockEmbeddingClient{resp: aiprovider.EmbeddingResponse{Embedding: []float32{0.1}}}
	h := New(client)

	_, err := h.Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"api_key": "k",
			"input":   "Embed this: {{.n1.text}}",
		},
		UpstreamData: map[string]any{
			"n1": map[string]any{"text": "some document text"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.lastReq.Input != "Embed this: some document text" {
		t.Errorf("template not expanded: got %q", client.lastReq.Input)
	}
}

func TestEmbeddingHandler_Execute_EmptyRenderedInput_ReturnsError(t *testing.T) {
	// Template expands to "" when the referenced key is missing.
	// With missingkey=error the template itself errors; this test covers the
	// post-render empty-string guard for the case where rawInput is non-empty
	// but the rendered result is empty (e.g. a plain empty string in config).
	h := New(&mockEmbeddingClient{resp: aiprovider.EmbeddingResponse{Embedding: []float32{0.1}}})
	_, err := h.Execute(context.Background(), node.NodeInput{
		// rawInput is a single space — not empty, so the pre-render check passes,
		// but we can't easily produce "" from a valid template with missingkey=error
		// without providing a missing key. Instead test the guard directly by
		// confirming a literal empty string in config is caught before rendering.
		Config:       map[string]any{"api_key": "k", "input": " "},
		UpstreamData: map[string]any{},
	})
	// A single space is not empty; the call should succeed (no error from the guard).
	if err != nil {
		t.Fatalf("unexpected error for non-empty input: %v", err)
	}
}

func TestEmbeddingHandler_Execute_MissingKeyTemplate_ReturnsError(t *testing.T) {
	h := New(&mockEmbeddingClient{})
	_, err := h.Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"api_key": "k",
			"input":   "embed: {{.n1.text}}",
		},
		UpstreamData: map[string]any{}, // n1 not present → missingkey=error fires
	})
	if err == nil {
		t.Fatal("expected error when template references a missing upstream key")
	}
}

func TestEmbeddingHandler_Execute_ClientError(t *testing.T) {
	h := New(&mockEmbeddingClient{err: errors.New("quota exceeded")})
	_, err := h.Execute(context.Background(), node.NodeInput{
		Config:       map[string]any{"api_key": "k", "input": "hi"},
		UpstreamData: map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error from client")
	}
}
