// Package rag_retrieve provides the built-in RAG Retrieve node for cogniflow.
// It embeds a query and retrieves the top-K most similar chunks using MySQL
// vector similarity search.
package rag_retrieve

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/g8rswimmer/cogniflow/internal/aiprovider"
	"github.com/g8rswimmer/cogniflow/internal/node"
	"github.com/g8rswimmer/cogniflow/internal/node/builtin/nodeutil"
	"github.com/g8rswimmer/cogniflow/internal/store"
)

const defaultTopK = 5

var inputSchema = json.RawMessage(`{
  "type": "object",
  "required": ["query"],
  "properties": {
    "api_key":     { "type": "string",  "title": "API Key",    "x-sensitive": true },
    "model":       { "type": "string",  "title": "Model",      "default": "text-embedding-3-small" },
    "query":       { "type": "string",  "title": "Query",      "x-template": true },
    "top_k":       { "type": "integer", "title": "Top K",      "default": 5 },
    "document_id": { "type": "string",  "title": "Document ID","x-template": true }
  }
}`)

var outputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "chunks": {
      "type": "array",
      "description": "Top-K most relevant chunks ordered by cosine distance (ascending).",
      "items": {
        "type": "object",
        "properties": {
          "id":         { "type": "string" },
          "chunk_text": { "type": "string" },
          "score":      { "type": "number", "description": "Cosine distance — lower is more similar." }
        }
      }
    }
  }
}`)

// retrieveStore is the store subset the RAG Retrieve node needs.
type retrieveStore interface {
	SearchChunks(ctx context.Context, embedding []float32, topK int, docFilter string) ([]store.RAGChunkResult, error)
}

// Handler implements the rag.retrieve built-in node.
type Handler struct {
	client aiprovider.EmbeddingClient
	store  retrieveStore
}

// New returns a new RAG Retrieve handler.
func New(client aiprovider.EmbeddingClient, st retrieveStore) *Handler {
	return &Handler{client: client, store: st}
}

func (h *Handler) Meta() node.NodeMeta {
	return node.NodeMeta{
		TypeID:       "rag.retrieve",
		DisplayName:  "RAG Retrieve",
		Category:     "ai",
		Description:  "Embed a query and retrieve the most relevant stored chunks using vector similarity search.",
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
	}
}

// Execute embeds the query and retrieves the top-K most similar chunks.
func (h *Handler) Execute(ctx context.Context, input node.NodeInput) (node.NodeOutput, error) {
	apiKey, _ := input.Config["api_key"].(string)
	model, _ := input.Config["model"].(string)

	rawQuery, _ := input.Config["query"].(string)
	if rawQuery == "" {
		return node.NodeOutput{}, fmt.Errorf("rag.retrieve: query is required")
	}
	query, err := nodeutil.RenderTemplate(rawQuery, input.UpstreamData)
	if err != nil {
		return node.NodeOutput{}, fmt.Errorf("rag.retrieve: render query: %w", err)
	}
	if query == "" {
		return node.NodeOutput{}, fmt.Errorf("rag.retrieve: rendered query is empty")
	}

	var docFilter string
	if raw, _ := input.Config["document_id"].(string); raw != "" {
		docFilter, err = nodeutil.RenderTemplate(raw, input.UpstreamData)
		if err != nil {
			return node.NodeOutput{}, fmt.Errorf("rag.retrieve: render document_id: %w", err)
		}
		if docFilter == "" {
			return node.NodeOutput{}, fmt.Errorf("rag.retrieve: document_id rendered to an empty string; check template references")
		}
	}

	topK := nodeutil.ToInt(input.Config["top_k"], defaultTopK)
	if topK <= 0 {
		return node.NodeOutput{}, fmt.Errorf("rag.retrieve: top_k must be a positive integer, got %d", topK)
	}

	resp, err := h.client.Embed(ctx, aiprovider.EmbeddingRequest{
		APIKey: apiKey,
		Model:  model,
		Input:  query,
	})
	if err != nil {
		return node.NodeOutput{}, fmt.Errorf("rag.retrieve: embed query: %w", err)
	}

	results, err := h.store.SearchChunks(ctx, resp.Embedding, topK, docFilter)
	if err != nil {
		return node.NodeOutput{}, fmt.Errorf("rag.retrieve: search: %w", err)
	}

	// Convert results to []any for JSON serialisation.
	chunks := make([]any, len(results))
	for i, r := range results {
		chunks[i] = map[string]any{
			"id":         r.ID,
			"chunk_text": r.ChunkText,
			"score":      r.Score,
		}
	}

	return node.NodeOutput{Data: map[string]any{
		"chunks": chunks,
	}}, nil
}
