package anthropic

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
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("unexpected api key header: %s", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("anthropic-version") == "" {
			t.Error("missing anthropic-version header")
		}
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"content": []map[string]any{
				{"type": "text", "text": "Hello from Claude!"},
			},
			"usage": map[string]any{"input_tokens": 12, "output_tokens": 4},
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
	if resp.Completion != "Hello from Claude!" {
		t.Errorf("want %q, got %q", "Hello from Claude!", resp.Completion)
	}
	if resp.PromptTokens != 12 || resp.CompletionTokens != 4 {
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

func TestComplete_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"authentication error"}}`)) //nolint:errcheck
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.Complete(context.Background(), aiprovider.LLMRequest{
		APIKey: "bad",
		Prompt: "hi",
	})
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
}

func TestComplete_SystemMessage(t *testing.T) {
	var gotPayload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotPayload) //nolint:errcheck
		json.NewEncoder(w).Encode(map[string]any{   //nolint:errcheck
			"content": []map[string]any{{"type": "text", "text": "ok"}},
			"usage":   map[string]any{"input_tokens": 1, "output_tokens": 1},
		})
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.Complete(context.Background(), aiprovider.LLMRequest{
		APIKey:    "k",
		Prompt:    "hi",
		SystemMsg: "You are helpful.",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPayload["system"] != "You are helpful." {
		t.Errorf("system message not sent: %v", gotPayload["system"])
	}
}

func TestComplete_NoTextContent_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate a tool_use-only response with no text block.
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"content": []map[string]any{
				{"type": "tool_use", "id": "toolu_01", "name": "search", "input": map[string]any{}},
			},
			"usage": map[string]any{"input_tokens": 5, "output_tokens": 3},
		})
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.Complete(context.Background(), aiprovider.LLMRequest{
		APIKey: "k",
		Prompt: "search for something",
	})
	if err == nil {
		t.Fatal("expected error when response has no text content block")
	}
}

func TestComplete_EmptyContent_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"content": []map[string]any{},
			"usage":   map[string]any{"input_tokens": 1, "output_tokens": 0},
		})
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.Complete(context.Background(), aiprovider.LLMRequest{
		APIKey: "k",
		Prompt: "hi",
	})
	if err == nil {
		t.Fatal("expected error for empty content array")
	}
}

func newTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	return New(WithTransport(&hostRewriteTransport{host: srv.URL[len("http://"):], inner: http.DefaultTransport}))
}

type hostRewriteTransport struct {
	host  string
	inner http.RoundTripper
}

func (t *hostRewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.URL.Scheme = "http"
	cloned.URL.Host = t.host
	return t.inner.RoundTrip(cloned)
}
