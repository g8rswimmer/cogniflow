// Package embedding provides the built-in Embedding node for cogniflow.
// It calls an aiprovider.EmbeddingClient and returns the embedding vector.
package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"text/template"

	"github.com/g8rswimmer/cogniflow/internal/aiprovider"
	"github.com/g8rswimmer/cogniflow/internal/node"
)

var inputSchema = json.RawMessage(`{
  "type": "object",
  "required": ["input"],
  "properties": {
    "api_key": { "type": "string", "title": "API Key",   "x-sensitive": true },
    "model":   { "type": "string", "title": "Model",     "default": "text-embedding-3-small" },
    "input":   { "type": "string", "title": "Input Text","x-template": true }
  }
}`)

var outputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "embedding": {
      "type": "array",
      "items": { "type": "number" },
      "description": "Float32 vector returned by the embedding model."
    }
  }
}`)

// Handler implements the embedding.openai built-in node.
type Handler struct {
	client aiprovider.EmbeddingClient
}

// New returns a new Embedding node handler backed by the given client.
func New(client aiprovider.EmbeddingClient) *Handler {
	return &Handler{client: client}
}

func (h *Handler) Meta() node.NodeMeta {
	return node.NodeMeta{
		TypeID:       "embedding.openai",
		DisplayName:  "Embedding (OpenAI)",
		Category:     "ai",
		Description:  "Generate a vector embedding for a text input using the OpenAI embeddings API.",
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
	}
}

// Execute renders the input template and calls the embedding provider.
func (h *Handler) Execute(ctx context.Context, input node.NodeInput) (node.NodeOutput, error) {
	apiKey, _ := input.Config["api_key"].(string)
	model, _ := input.Config["model"].(string)

	rawInput, _ := input.Config["input"].(string)
	if rawInput == "" {
		return node.NodeOutput{}, fmt.Errorf("embedding.openai: input is required")
	}

	rendered, err := renderTemplate(rawInput, input.UpstreamData)
	if err != nil {
		return node.NodeOutput{}, fmt.Errorf("embedding.openai: render input: %w", err)
	}
	if rendered == "" {
		return node.NodeOutput{}, fmt.Errorf("embedding.openai: rendered input is empty — check upstream template references")
	}

	resp, err := h.client.Embed(ctx, aiprovider.EmbeddingRequest{
		APIKey: apiKey,
		Model:  model,
		Input:  rendered,
	})
	if err != nil {
		return node.NodeOutput{}, fmt.Errorf("embedding.openai: %w", err)
	}

	// Convert []float32 to []any so it serialises cleanly to JSON.
	vec := make([]any, len(resp.Embedding))
	for i, v := range resp.Embedding {
		vec[i] = v
	}

	return node.NodeOutput{Data: map[string]any{
		"embedding": vec,
	}}, nil
}

func renderTemplate(s string, data map[string]any) (string, error) {
	t, err := template.New("").Option("missingkey=error").Parse(s)
	if err != nil {
		return s, err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return s, err
	}
	return buf.String(), nil
}
