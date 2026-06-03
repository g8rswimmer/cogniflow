package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/g8rswimmer/cogniflow/internal/aiprovider"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return New(srv.URL)
}

func TestClient_Embed_Success(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embed" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req["model"] != "nomic-embed-text" {
			t.Errorf("want model=nomic-embed-text, got %v", req["model"])
		}
		if req["input"] != "hello world" {
			t.Errorf("want input='hello world', got %v", req["input"])
		}
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"model":      "nomic-embed-text",
			"embeddings": [][]float32{{0.1, 0.2, 0.3}},
		})
	})

	resp, err := c.Embed(context.Background(), aiprovider.EmbeddingRequest{
		Model: "nomic-embed-text",
		Input: "hello world",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Embedding) != 3 {
		t.Errorf("want 3 dims, got %d", len(resp.Embedding))
	}
	if resp.Embedding[0] != 0.1 {
		t.Errorf("want first dim 0.1, got %f", resp.Embedding[0])
	}
}

func TestClient_Embed_DefaultModel(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck
		if req["model"] != defaultModel {
			t.Errorf("want default model=%s, got %v", defaultModel, req["model"])
		}
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"embeddings": [][]float32{{0.5}},
		})
	})

	_, err := c.Embed(context.Background(), aiprovider.EmbeddingRequest{Input: "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_Embed_IgnoresAPIKey(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("unexpected Authorization header set: %q", r.Header.Get("Authorization"))
		}
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"embeddings": [][]float32{{0.1}},
		})
	})

	_, err := c.Embed(context.Background(), aiprovider.EmbeddingRequest{
		APIKey: "should-be-ignored",
		Input:  "hello",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_Embed_HTTPError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "model not found", http.StatusNotFound)
	})

	_, err := c.Embed(context.Background(), aiprovider.EmbeddingRequest{Input: "hello"})
	if err == nil {
		t.Fatal("expected error for HTTP 404")
	}
}

func TestClient_Embed_ErrorField(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"error": "model nomic-embed-text not found, try pulling it first",
		})
	})

	_, err := c.Embed(context.Background(), aiprovider.EmbeddingRequest{Input: "hello"})
	if err == nil {
		t.Fatal("expected error from response error field")
	}
}

func TestClient_Embed_EmptyEmbeddings(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"embeddings": [][]float32{},
		})
	})

	_, err := c.Embed(context.Background(), aiprovider.EmbeddingRequest{Input: "hello"})
	if err == nil {
		t.Fatal("expected error for empty embeddings array")
	}
}

func TestNew_DefaultBaseURL(t *testing.T) {
	c := New("")
	if c.baseURL != defaultBaseURL {
		t.Errorf("want %s, got %s", defaultBaseURL, c.baseURL)
	}
}

func TestNew_StripsTrailingSlash(t *testing.T) {
	c := New("http://localhost:11434/")
	if c.baseURL != "http://localhost:11434" {
		t.Errorf("want trailing slash stripped, got %s", c.baseURL)
	}
}
