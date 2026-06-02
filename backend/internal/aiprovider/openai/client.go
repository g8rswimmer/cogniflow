// Package openai provides LLMClient and EmbeddingClient implementations backed
// by the OpenAI HTTP API. No third-party SDK is used; requests are made with
// the standard net/http client.
package openai

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
	chatEndpoint      = "https://api.openai.com/v1/chat/completions"
	embeddingEndpoint = "https://api.openai.com/v1/embeddings"

	// defaultTimeout is the client-level deadline for any single OpenAI request.
	// LLM completions on large prompts can be slow; 90 s is a conservative ceiling.
	defaultTimeout = 90 * time.Second
)

// Option configures the HTTP client used by the OpenAI Client.
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

// Client implements both aiprovider.LLMClient and aiprovider.EmbeddingClient
// for the OpenAI HTTP API.
type Client struct {
	http *http.Client
}

// New returns a new OpenAI Client with a default 90 s timeout.
// Pass Option values to override timeout or transport.
func New(opts ...Option) *Client {
	hc := &http.Client{Timeout: defaultTimeout}
	for _, o := range opts {
		o(hc)
	}
	return &Client{http: hc}
}

// ---- LLMClient --------------------------------------------------------------

func (c *Client) Complete(ctx context.Context, req aiprovider.LLMRequest) (aiprovider.LLMResponse, error) {
	if req.APIKey == "" {
		return aiprovider.LLMResponse{}, fmt.Errorf("openai: api_key is required")
	}
	if req.Model == "" {
		req.Model = "gpt-4o"
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1024
	}

	type message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	var messages []message
	if req.SystemMsg != "" {
		messages = append(messages, message{Role: "system", Content: req.SystemMsg})
	}
	messages = append(messages, message{Role: "user", Content: req.Prompt})

	payload := map[string]any{
		"model":      req.Model,
		"messages":   messages,
		"max_tokens": maxTokens,
	}
	// Only include temperature when the caller explicitly set it. A nil
	// Temperature means "use the provider's default" and is omitted from the
	// request so the OpenAI API applies its own default. Sending temperature=0
	// explicitly is valid and requests greedy/deterministic sampling.
	if req.Temperature != nil {
		payload["temperature"] = *req.Temperature
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return aiprovider.LLMResponse{}, fmt.Errorf("openai: marshal request: %w", err)
	}

	resp, err := c.doRequest(ctx, "POST", chatEndpoint, req.APIKey, body)
	if err != nil {
		return aiprovider.LLMResponse{}, err
	}
	defer resp.Body.Close()

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return aiprovider.LLMResponse{}, fmt.Errorf("openai: decode response: %w", err)
	}
	if result.Error != nil {
		return aiprovider.LLMResponse{}, fmt.Errorf("openai: %s", result.Error.Message)
	}
	if len(result.Choices) == 0 {
		return aiprovider.LLMResponse{}, fmt.Errorf("openai: no choices in response")
	}

	return aiprovider.LLMResponse{
		Completion:       result.Choices[0].Message.Content,
		PromptTokens:     result.Usage.PromptTokens,
		CompletionTokens: result.Usage.CompletionTokens,
	}, nil
}

// ---- EmbeddingClient --------------------------------------------------------

func (c *Client) Embed(ctx context.Context, req aiprovider.EmbeddingRequest) (aiprovider.EmbeddingResponse, error) {
	if req.APIKey == "" {
		return aiprovider.EmbeddingResponse{}, fmt.Errorf("openai: api_key is required")
	}
	if req.Model == "" {
		req.Model = "text-embedding-3-small"
	}

	body, err := json.Marshal(map[string]any{
		"model": req.Model,
		"input": req.Input,
	})
	if err != nil {
		return aiprovider.EmbeddingResponse{}, fmt.Errorf("openai: marshal embedding request: %w", err)
	}

	resp, err := c.doRequest(ctx, "POST", embeddingEndpoint, req.APIKey, body)
	if err != nil {
		return aiprovider.EmbeddingResponse{}, err
	}
	defer resp.Body.Close()

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return aiprovider.EmbeddingResponse{}, fmt.Errorf("openai: decode embedding response: %w", err)
	}
	if result.Error != nil {
		return aiprovider.EmbeddingResponse{}, fmt.Errorf("openai: %s", result.Error.Message)
	}
	if len(result.Data) == 0 {
		return aiprovider.EmbeddingResponse{}, fmt.Errorf("openai: no embeddings in response")
	}

	return aiprovider.EmbeddingResponse{Embedding: result.Data[0].Embedding}, nil
}

// ---- shared -----------------------------------------------------------------

func (c *Client) doRequest(ctx context.Context, method, url, apiKey string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai: http: %w", err)
	}
	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("openai: http %d: %s", resp.StatusCode, string(errBody))
	}
	return resp, nil
}
