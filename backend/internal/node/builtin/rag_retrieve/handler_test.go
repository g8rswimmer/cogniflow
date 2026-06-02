package rag_retrieve

import (
	"context"
	"errors"
	"testing"

	"github.com/g8rswimmer/cogniflow/internal/aiprovider"
	"github.com/g8rswimmer/cogniflow/internal/node"
	"github.com/g8rswimmer/cogniflow/internal/store"
)

// ---- mocks ------------------------------------------------------------------

type mockEmbedClient struct {
	resp    aiprovider.EmbeddingResponse
	err     error
	lastReq aiprovider.EmbeddingRequest
}

func (m *mockEmbedClient) Embed(_ context.Context, req aiprovider.EmbeddingRequest) (aiprovider.EmbeddingResponse, error) {
	m.lastReq = req
	return m.resp, m.err
}

type mockRetrieveStore struct {
	results []store.RAGChunkResult
	err     error
	lastK   int
	lastDoc string
}

func (m *mockRetrieveStore) SearchChunks(_ context.Context, _ []float32, topK int, docFilter string) ([]store.RAGChunkResult, error) {
	m.lastK = topK
	m.lastDoc = docFilter
	return m.results, m.err
}

// ---- tests ------------------------------------------------------------------

func TestRAGRetrieveHandler_Meta(t *testing.T) {
	h := New(&mockEmbedClient{}, &mockRetrieveStore{})
	meta := h.Meta()
	if meta.TypeID != "rag.retrieve" {
		t.Errorf("want rag.retrieve, got %s", meta.TypeID)
	}
	if meta.Category != "ai" {
		t.Errorf("want ai, got %s", meta.Category)
	}
}

func TestRAGRetrieveHandler_Execute_Success(t *testing.T) {
	client := &mockEmbedClient{resp: aiprovider.EmbeddingResponse{Embedding: []float32{0.1, 0.2}}}
	st := &mockRetrieveStore{
		results: []store.RAGChunkResult{
			{ID: "doc-1:0", ChunkText: "cogniflow is great", Score: 0.05},
			{ID: "doc-1:1", ChunkText: "it runs workflows", Score: 0.10},
		},
	}
	h := New(client, st)

	out, err := h.Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"api_key": "sk-test",
			"query":   "What is cogniflow?",
			"top_k":   2,
		},
		UpstreamData: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	chunks, ok := out.Data["chunks"].([]any)
	if !ok {
		t.Fatalf("chunks not []any: %T", out.Data["chunks"])
	}
	if len(chunks) != 2 {
		t.Errorf("want 2 chunks, got %d", len(chunks))
	}

	first, ok := chunks[0].(map[string]any)
	if !ok {
		t.Fatalf("chunk[0] not map[string]any")
	}
	if first["chunk_text"] != "cogniflow is great" {
		t.Errorf("want 'cogniflow is great', got %v", first["chunk_text"])
	}
	if st.lastK != 2 {
		t.Errorf("want top_k=2 passed to store, got %d", st.lastK)
	}
}

func TestRAGRetrieveHandler_Execute_DocFilter(t *testing.T) {
	client := &mockEmbedClient{resp: aiprovider.EmbeddingResponse{Embedding: []float32{0.1}}}
	st := &mockRetrieveStore{results: []store.RAGChunkResult{}}
	h := New(client, st)

	_, err := h.Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"query":       "question",
			"document_id": "my-doc",
		},
		UpstreamData: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.lastDoc != "my-doc" {
		t.Errorf("want document_id='my-doc' passed to store, got %q", st.lastDoc)
	}
}

func TestRAGRetrieveHandler_Execute_TemplateQuery(t *testing.T) {
	client := &mockEmbedClient{resp: aiprovider.EmbeddingResponse{Embedding: []float32{0.1}}}
	st := &mockRetrieveStore{}
	h := New(client, st)

	_, err := h.Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"query": "Find: {{._initial.question}}",
		},
		UpstreamData: map[string]any{
			"_initial": map[string]any{"question": "what is RAG?"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.lastReq.Input != "Find: what is RAG?" {
		t.Errorf("template not expanded: got %q", client.lastReq.Input)
	}
}

func TestRAGRetrieveHandler_Execute_DefaultTopK(t *testing.T) {
	client := &mockEmbedClient{resp: aiprovider.EmbeddingResponse{Embedding: []float32{0.1}}}
	st := &mockRetrieveStore{}
	h := New(client, st)

	_, err := h.Execute(context.Background(), node.NodeInput{
		Config:       map[string]any{"query": "hi"},
		UpstreamData: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.lastK != defaultTopK {
		t.Errorf("want default top_k=%d, got %d", defaultTopK, st.lastK)
	}
}

func TestRAGRetrieveHandler_Execute_ZeroTopK_ReturnsError(t *testing.T) {
	client := &mockEmbedClient{resp: aiprovider.EmbeddingResponse{Embedding: []float32{0.1}}}
	h := New(client, &mockRetrieveStore{})

	_, err := h.Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"query": "hi",
			"top_k": 0,
		},
		UpstreamData: map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error for top_k=0")
	}
}

func TestRAGRetrieveHandler_Execute_NegativeTopK_ReturnsError(t *testing.T) {
	client := &mockEmbedClient{resp: aiprovider.EmbeddingResponse{Embedding: []float32{0.1}}}
	h := New(client, &mockRetrieveStore{})

	_, err := h.Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"query": "hi",
			"top_k": -1,
		},
		UpstreamData: map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error for top_k=-1")
	}
}

func TestRAGRetrieveHandler_Execute_EmptyRenderedDocumentID_ReturnsError(t *testing.T) {
	client := &mockEmbedClient{resp: aiprovider.EmbeddingResponse{Embedding: []float32{0.1}}}
	h := New(client, &mockRetrieveStore{})

	// document_id is configured but renders to "" — must error, not silently drop the filter.
	_, err := h.Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"query":       "what is RAG?",
			"document_id": "{{._initial.scope}}",
		},
		UpstreamData: map[string]any{
			"_initial": map[string]any{"scope": ""},
		},
	})
	if err == nil {
		t.Fatal("expected error when document_id renders to empty string")
	}
}

func TestRAGRetrieveHandler_Execute_MissingQuery(t *testing.T) {
	h := New(&mockEmbedClient{}, &mockRetrieveStore{})
	_, err := h.Execute(context.Background(), node.NodeInput{
		Config:       map[string]any{},
		UpstreamData: map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error for missing query")
	}
}

func TestRAGRetrieveHandler_Execute_EmbedError(t *testing.T) {
	client := &mockEmbedClient{err: errors.New("api error")}
	h := New(client, &mockRetrieveStore{})

	_, err := h.Execute(context.Background(), node.NodeInput{
		Config:       map[string]any{"query": "hello"},
		UpstreamData: map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error from embed client")
	}
}

func TestRAGRetrieveHandler_Execute_SearchError(t *testing.T) {
	client := &mockEmbedClient{resp: aiprovider.EmbeddingResponse{Embedding: []float32{0.1}}}
	st := &mockRetrieveStore{err: errors.New("db down")}
	h := New(client, st)

	_, err := h.Execute(context.Background(), node.NodeInput{
		Config:       map[string]any{"query": "hello"},
		UpstreamData: map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error from store")
	}
}

func TestRAGRetrieveHandler_Execute_EmptyResults(t *testing.T) {
	client := &mockEmbedClient{resp: aiprovider.EmbeddingResponse{Embedding: []float32{0.1}}}
	st := &mockRetrieveStore{results: []store.RAGChunkResult{}}
	h := New(client, st)

	out, err := h.Execute(context.Background(), node.NodeInput{
		Config:       map[string]any{"query": "unknown topic"},
		UpstreamData: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	chunks, _ := out.Data["chunks"].([]any)
	if len(chunks) != 0 {
		t.Errorf("want empty chunks, got %d", len(chunks))
	}
}
