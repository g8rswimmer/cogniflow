package rag_ingest

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/g8rswimmer/cogniflow/internal/aiprovider"
	"github.com/g8rswimmer/cogniflow/internal/node"
	"github.com/g8rswimmer/cogniflow/internal/store"
)

// ---- mocks ------------------------------------------------------------------

type mockEmbedClient struct {
	resp    aiprovider.EmbeddingResponse
	err     error
	calls   int
	lastReq aiprovider.EmbeddingRequest
}

func (m *mockEmbedClient) Embed(_ context.Context, req aiprovider.EmbeddingRequest) (aiprovider.EmbeddingResponse, error) {
	m.calls++
	m.lastReq = req
	return m.resp, m.err
}

type mockChunkStore struct {
	upserted []store.RAGChunk
	err      error
}

func (m *mockChunkStore) UpsertChunks(_ context.Context, chunks []store.RAGChunk) error {
	m.upserted = append(m.upserted, chunks...)
	return m.err
}

// ---- tests ------------------------------------------------------------------

func TestRAGIngestHandler_Meta(t *testing.T) {
	h := New(&mockEmbedClient{}, &mockChunkStore{})
	meta := h.Meta()
	if meta.TypeID != "rag.ingest" {
		t.Errorf("want rag.ingest, got %s", meta.TypeID)
	}
	if meta.Category != "ai" {
		t.Errorf("want ai, got %s", meta.Category)
	}
}

func TestRAGIngestHandler_Execute_Success(t *testing.T) {
	vec := []float32{0.1, 0.2, 0.3}
	client := &mockEmbedClient{resp: aiprovider.EmbeddingResponse{Embedding: vec}}
	st := &mockChunkStore{}
	h := New(client, st)

	out, err := h.Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"api_key":     "sk-test",
			"text":        "Hello world",
			"document_id": "doc-1",
		},
		UpstreamData: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Data["chunks_ingested"].(int) != 1 {
		t.Errorf("want 1 chunk, got %v", out.Data["chunks_ingested"])
	}
	if out.Data["document_id"] != "doc-1" {
		t.Errorf("want doc-1, got %v", out.Data["document_id"])
	}
	if len(st.upserted) != 1 {
		t.Fatalf("want 1 upserted chunk, got %d", len(st.upserted))
	}
	if st.upserted[0].DocumentID != "doc-1" {
		t.Errorf("want document_id doc-1, got %s", st.upserted[0].DocumentID)
	}
	if st.upserted[0].ChunkText != "Hello world" {
		t.Errorf("unexpected chunk text: %s", st.upserted[0].ChunkText)
	}
}

func TestRAGIngestHandler_Execute_MultipleChunks(t *testing.T) {
	client := &mockEmbedClient{resp: aiprovider.EmbeddingResponse{Embedding: []float32{0.1}}}
	st := &mockChunkStore{}
	h := New(client, st)

	// 20-char text, chunk_size=10, overlap=0 → 2 chunks
	out, err := h.Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"text":          "01234567890123456789",
			"document_id":   "doc-multi",
			"chunk_size":    10,
			"chunk_overlap": 0,
		},
		UpstreamData: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Data["chunks_ingested"].(int) != 2 {
		t.Errorf("want 2 chunks, got %v", out.Data["chunks_ingested"])
	}
	if client.calls != 2 {
		t.Errorf("want 2 embedding calls, got %d", client.calls)
	}
	if len(st.upserted) != 2 {
		t.Fatalf("want 2 upserted chunks, got %d", len(st.upserted))
	}
}

func TestRAGIngestHandler_Execute_TemplateText(t *testing.T) {
	client := &mockEmbedClient{resp: aiprovider.EmbeddingResponse{Embedding: []float32{0.1}}}
	st := &mockChunkStore{}
	h := New(client, st)

	_, err := h.Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"text":        "{{._initial.body}}",
			"document_id": "doc-tmpl",
		},
		UpstreamData: map[string]any{
			"_initial": map[string]any{"body": "template resolved text"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.upserted[0].ChunkText != "template resolved text" {
		t.Errorf("unexpected chunk text: %q", st.upserted[0].ChunkText)
	}
}

func TestRAGIngestHandler_Execute_MissingDocumentID_ReturnsError(t *testing.T) {
	h := New(&mockEmbedClient{resp: aiprovider.EmbeddingResponse{Embedding: []float32{0.1}}}, &mockChunkStore{})

	_, err := h.Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"text": "some text",
			// no document_id — every run would write to a fresh key with no cleanup path
		},
		UpstreamData: map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error when document_id is missing")
	}
	if !strings.Contains(err.Error(), "document_id is required") {
		t.Errorf("expected 'document_id is required' in error, got: %v", err)
	}
}

func TestRAGIngestHandler_Execute_EmptyRenderedDocumentID_ReturnsError(t *testing.T) {
	client := &mockEmbedClient{resp: aiprovider.EmbeddingResponse{Embedding: []float32{0.1}}}
	h := New(client, &mockChunkStore{})

	// document_id is configured but renders to "" — must return an explicit error.
	_, err := h.Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"text":        "hello",
			"document_id": "{{._initial.id}}",
		},
		UpstreamData: map[string]any{
			"_initial": map[string]any{"id": ""},
		},
	})
	if err == nil {
		t.Fatal("expected error when document_id renders to empty string")
	}
	if !strings.Contains(err.Error(), "empty string") {
		t.Errorf("expected 'empty string' in error, got: %v", err)
	}
}

func TestRAGIngestHandler_Execute_LongDocumentID_ReturnsError(t *testing.T) {
	client := &mockEmbedClient{resp: aiprovider.EmbeddingResponse{Embedding: []float32{0.1}}}
	h := New(client, &mockChunkStore{})

	longID := strings.Repeat("x", maxDocIDLen+1)
	_, err := h.Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"text":        "hello",
			"document_id": longID,
		},
		UpstreamData: map[string]any{},
	})
	if err == nil {
		t.Fatalf("expected error for document_id longer than %d bytes", maxDocIDLen)
	}
	if !strings.Contains(err.Error(), "maximum length") {
		t.Errorf("expected 'maximum length' in error, got: %v", err)
	}
}

func TestRAGIngestHandler_Execute_MissingText(t *testing.T) {
	h := New(&mockEmbedClient{}, &mockChunkStore{})
	_, err := h.Execute(context.Background(), node.NodeInput{
		Config:       map[string]any{},
		UpstreamData: map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error for missing text")
	}
}

func TestRAGIngestHandler_Execute_EmptyEmbedding_ReturnsError(t *testing.T) {
	// Provider returns an empty embedding slice — should be caught before store write.
	client := &mockEmbedClient{resp: aiprovider.EmbeddingResponse{Embedding: []float32{}}}
	h := New(client, &mockChunkStore{})

	_, err := h.Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"text":        "hello",
			"document_id": "doc-empty-emb",
		},
		UpstreamData: map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error when embedding is empty")
	}
	if !strings.Contains(err.Error(), "empty embedding") {
		t.Errorf("expected 'empty embedding' in error, got: %v", err)
	}
}

func TestRAGIngestHandler_Execute_EmbedError(t *testing.T) {
	client := &mockEmbedClient{err: errors.New("quota exceeded")}
	h := New(client, &mockChunkStore{})

	_, err := h.Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"text":        "hello",
			"document_id": "doc-x",
		},
		UpstreamData: map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error from embedding client")
	}
}

func TestRAGIngestHandler_Execute_StoreError(t *testing.T) {
	client := &mockEmbedClient{resp: aiprovider.EmbeddingResponse{Embedding: []float32{0.1}}}
	st := &mockChunkStore{err: errors.New("db down")}
	h := New(client, st)

	_, err := h.Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"text":        "hello",
			"document_id": "doc-err",
		},
		UpstreamData: map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error from store")
	}
}

func TestChunkText_SingleChunk(t *testing.T) {
	chunks := chunkText("hello", 512, 50)
	if len(chunks) != 1 {
		t.Fatalf("want 1 chunk, got %d", len(chunks))
	}
	if chunks[0] != "hello" {
		t.Errorf("want 'hello', got %q", chunks[0])
	}
}

func TestChunkText_MultipleChunks(t *testing.T) {
	chunks := chunkText("0123456789", 4, 0)
	// 10 chars / size 4, no overlap → chunks: [0-3], [4-7], [8-9]
	if len(chunks) != 3 {
		t.Fatalf("want 3 chunks, got %d: %v", len(chunks), chunks)
	}
	if chunks[0] != "0123" {
		t.Errorf("chunk 0: want '0123', got %q", chunks[0])
	}
	if chunks[1] != "4567" {
		t.Errorf("chunk 1: want '4567', got %q", chunks[1])
	}
	if chunks[2] != "89" {
		t.Errorf("chunk 2: want '89', got %q", chunks[2])
	}
}

func TestChunkText_WithOverlap(t *testing.T) {
	chunks := chunkText("0123456789", 6, 2)
	// step = 6-2 = 4: [0-5], [4-9]
	if len(chunks) != 2 {
		t.Fatalf("want 2 chunks, got %d: %v", len(chunks), chunks)
	}
	if chunks[0] != "012345" {
		t.Errorf("chunk 0: want '012345', got %q", chunks[0])
	}
	if chunks[1] != "456789" {
		t.Errorf("chunk 1: want '456789', got %q", chunks[1])
	}
}

func TestChunkText_Empty(t *testing.T) {
	if chunks := chunkText("", 512, 50); chunks != nil {
		t.Errorf("expected nil for empty text, got %v", chunks)
	}
}

func TestChunkText_OverlapExceedsSize(t *testing.T) {
	// overlap >= chunk_size → clamped to chunkSize/2
	chunks := chunkText("0123456789", 4, 10)
	if len(chunks) == 0 {
		t.Fatal("expected chunks, got none")
	}
}
