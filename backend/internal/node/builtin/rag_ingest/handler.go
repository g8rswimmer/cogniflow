// Package rag_ingest provides the built-in RAG Ingest node for cogniflow.
// It chunks input text, generates embeddings for each chunk, and stores them
// in the configured store for later retrieval.
package rag_ingest

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"strconv"

	"github.com/g8rswimmer/cogniflow/internal/aiprovider"
	"github.com/g8rswimmer/cogniflow/internal/node"
	"github.com/g8rswimmer/cogniflow/internal/node/builtin/nodeutil"
	"github.com/g8rswimmer/cogniflow/internal/store"
)

const (
	defaultChunkSize    = 512
	defaultChunkOverlap = 50
	// maxDocIDLen is the maximum allowed length for a document_id. Chunk IDs are
	// constructed as documentID+":"+chunkIndex; the rag_chunks.id column is
	// VARCHAR(255), so document IDs longer than this would overflow the PK.
	maxDocIDLen = 200
)

var inputSchema = json.RawMessage(`{
  "type": "object",
  "required": ["text"],
  "properties": {
    "api_key":       { "type": "string",  "title": "API Key",       "x-sensitive": true },
    "model":         { "type": "string",  "title": "Model",         "default": "text-embedding-3-small" },
    "text":          { "type": "string",  "title": "Text",          "x-template": true },
    "document_id":   { "type": "string",  "title": "Document ID",   "x-template": true },
    "chunk_size":    { "type": "integer", "title": "Chunk Size",    "default": 512 },
    "chunk_overlap": { "type": "integer", "title": "Chunk Overlap", "default": 50 }
  }
}`)

var outputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "chunks_ingested": { "type": "integer", "description": "Number of chunks stored." },
    "document_id":     { "type": "string",  "description": "Document ID used for ingestion." }
  }
}`)

// chunkStore is the store subset the RAG Ingest node needs.
type chunkStore interface {
	UpsertChunks(ctx context.Context, chunks []store.RAGChunk) error
}

// Handler implements the rag.ingest built-in node.
type Handler struct {
	client aiprovider.EmbeddingClient
	store  chunkStore
}

// New returns a new RAG Ingest handler.
func New(client aiprovider.EmbeddingClient, st chunkStore) *Handler {
	return &Handler{client: client, store: st}
}

func (h *Handler) Meta() node.NodeMeta {
	return node.NodeMeta{
		TypeID:       "rag.ingest",
		DisplayName:  "RAG Ingest",
		Category:     "ai",
		Description:  "Chunk and embed text, then store the vectors for retrieval-augmented generation.",
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
	}
}

// Execute chunks the input text, generates an embedding for each chunk,
// and upserts all chunks to the store.
func (h *Handler) Execute(ctx context.Context, input node.NodeInput) (node.NodeOutput, error) {
	apiKey, _ := input.Config["api_key"].(string)
	model, _ := input.Config["model"].(string)

	rawText, _ := input.Config["text"].(string)
	if rawText == "" {
		return node.NodeOutput{}, fmt.Errorf("rag.ingest: text is required")
	}
	text, err := nodeutil.RenderTemplate(rawText, input.UpstreamData)
	if err != nil {
		return node.NodeOutput{}, fmt.Errorf("rag.ingest: render text: %w", err)
	}
	if text == "" {
		return node.NodeOutput{}, fmt.Errorf("rag.ingest: rendered text is empty")
	}

	docID, err := resolveDocumentID(input)
	if err != nil {
		return node.NodeOutput{}, err
	}

	chunkSize := nodeutil.ToInt(input.Config["chunk_size"], defaultChunkSize)
	chunkOverlap := nodeutil.ToInt(input.Config["chunk_overlap"], defaultChunkOverlap)

	textChunks := chunkText(text, chunkSize, chunkOverlap)
	if len(textChunks) == 0 {
		return node.NodeOutput{}, fmt.Errorf("rag.ingest: text produced no chunks")
	}

	ragChunks := make([]store.RAGChunk, 0, len(textChunks))
	for i, chunk := range textChunks {
		resp, err := h.client.Embed(ctx, aiprovider.EmbeddingRequest{
			APIKey: apiKey,
			Model:  model,
			Input:  chunk,
		})
		if err != nil {
			return node.NodeOutput{}, fmt.Errorf("rag.ingest: embed chunk %d: %w", i, err)
		}
		if len(resp.Embedding) == 0 {
			return node.NodeOutput{}, fmt.Errorf("rag.ingest: embed chunk %d: provider returned empty embedding", i)
		}
		ragChunks = append(ragChunks, store.RAGChunk{
			ID:         docID + ":" + strconv.Itoa(i),
			DocumentID: docID,
			ChunkIndex: i,
			ChunkText:  chunk,
			Embedding:  resp.Embedding,
		})
	}

	if err := h.store.UpsertChunks(ctx, ragChunks); err != nil {
		return node.NodeOutput{}, fmt.Errorf("rag.ingest: upsert chunks: %w", err)
	}

	return node.NodeOutput{Data: map[string]any{
		"chunks_ingested": len(ragChunks),
		"document_id":     docID,
	}}, nil
}

// resolveDocumentID returns the document ID for this run:
//   - If document_id is not configured, a random UUID is generated.
//   - If document_id is configured, the template is expanded; an empty result
//     after expansion is an explicit error to prevent silently orphaning chunks.
//   - IDs longer than maxDocIDLen are rejected to prevent VARCHAR(255) overflow
//     when the chunk PK is constructed as documentID+":"+chunkIndex.
func resolveDocumentID(input node.NodeInput) (string, error) {
	raw, _ := input.Config["document_id"].(string)
	if raw == "" {
		return newID(), nil
	}
	docID, err := nodeutil.RenderTemplate(raw, input.UpstreamData)
	if err != nil {
		return "", fmt.Errorf("rag.ingest: render document_id: %w", err)
	}
	if docID == "" {
		return "", fmt.Errorf("rag.ingest: document_id rendered to an empty string; check template references")
	}
	if len(docID) > maxDocIDLen {
		return "", fmt.Errorf("rag.ingest: document_id exceeds maximum length of %d characters (got %d)", maxDocIDLen, len(docID))
	}
	return docID, nil
}

// chunkText splits text into overlapping chunks of chunkSize runes.
func chunkText(text string, chunkSize, overlap int) []string {
	if chunkSize <= 0 {
		chunkSize = defaultChunkSize
	}
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= chunkSize {
		overlap = chunkSize / 2
	}

	runes := []rune(text)
	if len(runes) == 0 {
		return nil
	}

	step := chunkSize - overlap
	if step <= 0 {
		step = 1
	}

	var chunks []string
	for start := 0; start < len(runes); start += step {
		end := start + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[start:end]))
		if end >= len(runes) {
			break
		}
	}
	return chunks
}

func newID() string {
	var b [16]byte
	if _, err := io.ReadFull(rand.Reader, b[:]); err != nil {
		panic(fmt.Sprintf("rag_ingest: read random: %v", err))
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
