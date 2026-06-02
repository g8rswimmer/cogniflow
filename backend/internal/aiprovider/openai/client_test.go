package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/g8rswimmer/cogniflow/internal/aiprovider"
)

func TestComplete_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected auth: %s", r.Header.Get("Authorization"))
		}
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"choices": []map[string]any{
				{"message": map[string]any{"content": "Hello!"}},
			},
			"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 5},
		})
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	resp, err := c.Complete(context.Background(), aiprovider.LLMRequest{
		APIKey: "test-key",
		Prompt: "Say hello.",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Completion != "Hello!" {
		t.Errorf("want Hello!, got %q", resp.Completion)
	}
	if resp.PromptTokens != 10 || resp.CompletionTokens != 5 {
		t.Errorf("unexpected token counts: %+v", resp)
	}
}

func TestComplete_MissingAPIKey(t *testing.T) {
	c := New()
	_, err := c.Complete(context.Background(), aiprovider.LLMRequest{Prompt: "hi"})
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}

func TestComplete_TemperatureZeroIsForwarded(t *testing.T) {
	var gotPayload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotPayload) //nolint:errcheck
		json.NewEncoder(w).Encode(map[string]any{   //nolint:errcheck
			"choices": []map[string]any{{"message": map[string]any{"content": "ok"}}},
			"usage":   map[string]any{"prompt_tokens": 1, "completion_tokens": 1},
		})
	}))
	defer srv.Close()

	zero := 0.0
	c := newTestClient(t, srv)
	_, err := c.Complete(context.Background(), aiprovider.LLMRequest{
		APIKey:      "k",
		Prompt:      "hi",
		Temperature: &zero,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if temp, ok := gotPayload["temperature"]; !ok || temp != 0.0 {
		t.Errorf("want temperature=0.0 forwarded to API, got %v (present=%v)", temp, ok)
	}
}

func TestComplete_TemperatureNilIsOmitted(t *testing.T) {
	var gotPayload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotPayload) //nolint:errcheck
		json.NewEncoder(w).Encode(map[string]any{   //nolint:errcheck
			"choices": []map[string]any{{"message": map[string]any{"content": "ok"}}},
			"usage":   map[string]any{"prompt_tokens": 1, "completion_tokens": 1},
		})
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.Complete(context.Background(), aiprovider.LLMRequest{
		APIKey:      "k",
		Prompt:      "hi",
		Temperature: nil,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, present := gotPayload["temperature"]; present {
		t.Error("nil Temperature must not send temperature field to API")
	}
}

func TestComplete_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"invalid api key"}}`)) //nolint:errcheck
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.Complete(context.Background(), aiprovider.LLMRequest{
		APIKey: "bad-key",
		Prompt: "hi",
	})
	if err == nil {
		t.Fatal("expected error for API error response")
	}
}

func TestEmbed_Success(t *testing.T) {
	want := []float32{0.1, 0.2, 0.3}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"data": []map[string]any{
				{"embedding": want},
			},
		})
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	resp, err := c.Embed(context.Background(), aiprovider.EmbeddingRequest{
		APIKey: "test-key",
		Input:  "hello world",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Embedding) != len(want) {
		t.Fatalf("want embedding length %d, got %d", len(want), len(resp.Embedding))
	}
}

func TestEmbed_MissingAPIKey(t *testing.T) {
	c := New()
	_, err := c.Embed(context.Background(), aiprovider.EmbeddingRequest{Input: "hi"})
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}

// newTestClient creates a Client that redirects all requests to srv.
func newTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	return New(WithTransport(&pathRewriteTransport{base: srv.URL, inner: http.DefaultTransport}))
}

// pathRewriteTransport rewrites all requests to point at a test server.
type pathRewriteTransport struct {
	base  string
	inner http.RoundTripper
}

func (t *pathRewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.URL.Scheme = "http"
	cloned.URL.Host = t.base[len("http://"):]
	return t.inner.RoundTrip(cloned)
}
