// Package anthropic provides an LLMClient implementation backed by the
// Anthropic Messages HTTP API. No third-party SDK is used.
package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/g8rswimmer/cogniflow/internal/aiprovider"
)

const (
	messagesEndpoint = "https://api.anthropic.com/v1/messages"

	// defaultTimeout is the client-level deadline for any single Anthropic request.
	defaultTimeout = 90 * time.Second
)

// Option configures the HTTP client used by the Anthropic Client.
type Option func(*http.Client)

// WithTimeout overrides the default request timeout.
func WithTimeout(d time.Duration) Option {
	return func(hc *http.Client) { hc.Timeout = d }
}

// WithTransport replaces the HTTP transport. Primarily used in tests to redirect
// requests to a local test server.
func WithTransport(rt http.RoundTripper) Option {
	return func(hc *http.Client) { hc.Transport = rt }
}

// Client implements aiprovider.LLMClient for the Anthropic Messages API.
type Client struct {
	http *http.Client
}

// New returns a new Anthropic Client with a default 90 s timeout.
// Pass Option values to override timeout or transport.
func New(opts ...Option) *Client {
	hc := &http.Client{Timeout: defaultTimeout}
	for _, o := range opts {
		o(hc)
	}
	return &Client{http: hc}
}

// Complete calls the Anthropic Messages API and returns the text response.
func (c *Client) Complete(ctx context.Context, req aiprovider.LLMRequest) (aiprovider.LLMResponse, error) {
	if req.APIKey == "" {
		return aiprovider.LLMResponse{}, fmt.Errorf("anthropic: api_key is required")
	}
	if req.Model == "" {
		req.Model = "claude-sonnet-4-6"
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1024
	}

	payload := map[string]any{
		"model":      req.Model,
		"max_tokens": maxTokens,
		"messages": []map[string]any{
			{"role": "user", "content": req.Prompt},
		},
	}
	if req.SystemMsg != "" {
		payload["system"] = req.SystemMsg
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return aiprovider.LLMResponse{}, fmt.Errorf("anthropic: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", messagesEndpoint, bytes.NewReader(body))
	if err != nil {
		return aiprovider.LLMResponse{}, fmt.Errorf("anthropic: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", req.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return aiprovider.LLMResponse{}, fmt.Errorf("anthropic: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return aiprovider.LLMResponse{}, fmt.Errorf("anthropic: http %d: %s", resp.StatusCode, string(b))
	}

	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return aiprovider.LLMResponse{}, fmt.Errorf("anthropic: decode response: %w", err)
	}
	if result.Error != nil {
		return aiprovider.LLMResponse{}, fmt.Errorf("anthropic: %s", result.Error.Message)
	}

	var completion string
	for _, block := range result.Content {
		if block.Type == "text" {
			completion = block.Text
			break
		}
	}
	// Guard against responses that contain no text block (e.g. a tool_use-only
	// response). Returning an empty completion with no error would silently
	// produce blank output for the node and all downstream nodes.
	if completion == "" {
		return aiprovider.LLMResponse{}, fmt.Errorf("anthropic: response contained no text content (%d content blocks)", len(result.Content))
	}

	return aiprovider.LLMResponse{
		Completion:       completion,
		PromptTokens:     result.Usage.InputTokens,
		CompletionTokens: result.Usage.OutputTokens,
	}, nil
}
