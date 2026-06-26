// Package ollama provides an EmbeddingClient implementation backed by a local
// Ollama server. No API key is required; authentication is not supported by
// Ollama's default configuration.
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/g8rswimmer/cogniflow/internal/aiprovider"
	"github.com/g8rswimmer/cogniflow/internal/aiprovider/httpclient"
)

const (
	defaultBaseURL = "http://localhost:11434"
	// defaultTimeout is generous because the first call to a model may trigger
	// a model load which can take several seconds on cold start.
	defaultTimeout = 120 * time.Second
	defaultModel   = "nomic-embed-text"
)

// Option configures the Ollama Client.
type Option func(*Client)

// WithTimeout overrides the default request timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) { c.http.Timeout = d }
}

// WithTransport replaces the HTTP transport. Primarily used in tests to redirect
// requests to a local test server.
func WithTransport(rt http.RoundTripper) Option {
	return func(c *Client) { c.http.Transport = rt }
}

// Client implements aiprovider.EmbeddingClient for a local Ollama server.
// EmbeddingRequest.APIKey is silently ignored — Ollama requires no authentication
// in its default configuration.
type Client struct {
	http    *http.Client
	baseURL string
}

// New returns a new Ollama Client targeting baseURL (e.g. "http://localhost:11434").
// If baseURL is empty, the default "http://localhost:11434" is used. A retry
// transport is applied by default (3 attempts, 1 s base delay); WithTransport
// bypasses it.
func New(baseURL string, opts ...Option) *Client {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	c := &Client{
		http: &http.Client{
			Timeout:   defaultTimeout,
			Transport: httpclient.NewRetryTransport(nil, 3, time.Second),
		},
		baseURL: strings.TrimRight(baseURL, "/"),
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Embed generates an embedding for req.Input using the Ollama /api/embed endpoint.
// req.APIKey is ignored. req.Model should be an Ollama model name
// (e.g. "nomic-embed-text"); if empty, "nomic-embed-text" is used.
func (c *Client) Embed(ctx context.Context, req aiprovider.EmbeddingRequest) (aiprovider.EmbeddingResponse, error) {
	if req.Model == "" {
		req.Model = defaultModel
	}

	body, err := json.Marshal(map[string]any{
		"model": req.Model,
		"input": req.Input,
	})
	if err != nil {
		return aiprovider.EmbeddingResponse{}, fmt.Errorf("ollama: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return aiprovider.EmbeddingResponse{}, fmt.Errorf("ollama: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return aiprovider.EmbeddingResponse{}, fmt.Errorf("ollama: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		const errBodyLimit = 1 << 20 // 1 MiB cap; Ollama error bodies are small
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, errBodyLimit))
		return aiprovider.EmbeddingResponse{}, fmt.Errorf("ollama: http %d: %s", resp.StatusCode, strings.TrimSpace(string(errBody)))
	}

	var result struct {
		Embeddings [][]float32 `json:"embeddings"`
		Error      string      `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return aiprovider.EmbeddingResponse{}, fmt.Errorf("ollama: decode response: %w", err)
	}
	if result.Error != "" {
		return aiprovider.EmbeddingResponse{}, fmt.Errorf("ollama: %s", result.Error)
	}
	if len(result.Embeddings) == 0 || len(result.Embeddings[0]) == 0 {
		return aiprovider.EmbeddingResponse{}, fmt.Errorf("ollama: no embeddings in response")
	}

	return aiprovider.EmbeddingResponse{Embedding: result.Embeddings[0]}, nil
}
