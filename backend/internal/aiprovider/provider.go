// Package aiprovider defines the LLMClient and EmbeddingClient interfaces and
// the request/response types shared by all AI provider implementations.
package aiprovider

import "context"

// LLMRequest carries all parameters for a single language-model call.
// APIKey is per-call because each workflow node stores its own credential.
//
// Temperature is a pointer so that callers can distinguish "explicitly set to
// zero" (greedy/deterministic sampling) from "not configured" (use the
// provider's own default). A nil Temperature causes the provider client to
// omit the field from the request entirely.
type LLMRequest struct {
	APIKey      string
	Model       string
	SystemMsg   string
	Prompt      string
	MaxTokens   int
	Temperature *float64
}

// LLMResponse holds the result of a completed language-model call.
type LLMResponse struct {
	Completion       string
	PromptTokens     int
	CompletionTokens int
}

// LLMClient calls a language model and returns a completion.
type LLMClient interface {
	Complete(ctx context.Context, req LLMRequest) (LLMResponse, error)
}

// EmbeddingRequest carries all parameters for a single embedding call.
type EmbeddingRequest struct {
	APIKey string
	Model  string
	Input  string
}

// EmbeddingResponse holds the result of a completed embedding call.
type EmbeddingResponse struct {
	Embedding []float32
}

// EmbeddingClient generates vector embeddings for text inputs.
type EmbeddingClient interface {
	Embed(ctx context.Context, req EmbeddingRequest) (EmbeddingResponse, error)
}
