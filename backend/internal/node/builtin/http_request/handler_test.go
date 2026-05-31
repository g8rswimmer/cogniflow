package httprequest_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	httprequest "github.com/g8rswimmer/cogniflow/internal/node/builtin/http_request"
	"github.com/g8rswimmer/cogniflow/internal/node"
)

func TestHandler_Meta_TypeID(t *testing.T) {
	h := httprequest.New()
	if h.Meta().TypeID != "http.request" {
		t.Fatalf("expected type_id 'http.request', got %q", h.Meta().TypeID)
	}
}

func TestHandler_Meta_Category(t *testing.T) {
	h := httprequest.New()
	if h.Meta().Category != "deterministic" {
		t.Fatalf("expected category 'deterministic', got %q", h.Meta().Category)
	}
}

func TestHandler_Meta_DisplayName(t *testing.T) {
	h := httprequest.New()
	if h.Meta().DisplayName == "" {
		t.Fatal("display_name should not be empty")
	}
}

func TestHandler_Meta_Description(t *testing.T) {
	h := httprequest.New()
	if h.Meta().Description == "" {
		t.Fatal("description should not be empty")
	}
}

func TestHandler_Meta_InputSchema_Valid(t *testing.T) {
	h := httprequest.New()
	var schema map[string]any
	if err := json.Unmarshal(h.Meta().InputSchema, &schema); err != nil {
		t.Fatalf("input_schema is not valid JSON: %v", err)
	}
}

func TestHandler_Meta_OutputSchema_Valid(t *testing.T) {
	h := httprequest.New()
	var schema map[string]any
	if err := json.Unmarshal(h.Meta().OutputSchema, &schema); err != nil {
		t.Fatalf("output_schema is not valid JSON: %v", err)
	}
}

func TestHandler_Meta_InputSchema_HasRequiredFields(t *testing.T) {
	h := httprequest.New()
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if err := json.Unmarshal(h.Meta().InputSchema, &schema); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	if _, ok := schema.Properties["url"]; !ok {
		t.Error("input_schema missing 'url' property")
	}
	if _, ok := schema.Properties["method"]; !ok {
		t.Error("input_schema missing 'method' property")
	}
}

func TestHandler_ImplementsNodeHandler(t *testing.T) {
	var _ node.NodeHandler = httprequest.New()
}

func TestHandler_Execute_GET(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	h := httprequest.New()
	out, err := h.Execute(context.Background(), node.NodeInput{
		Config:       map[string]any{"url": srv.URL, "method": "GET"},
		UpstreamData: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Data["status_code"] != 200 {
		t.Errorf("expected status_code=200, got %v", out.Data["status_code"])
	}
	if out.Data["body"] != `{"ok":true}` {
		t.Errorf("unexpected body: %v", out.Data["body"])
	}
}

func TestHandler_Execute_POST_WithBody(t *testing.T) {
	var receivedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		receivedBody = buf[:n]
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	h := httprequest.New()
	out, err := h.Execute(context.Background(), node.NodeInput{
		Config:       map[string]any{"url": srv.URL, "method": "POST", "body": `{"msg":"hello"}`},
		UpstreamData: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Data["status_code"] != 201 {
		t.Errorf("expected 201, got %v", out.Data["status_code"])
	}
	if string(receivedBody) != `{"msg":"hello"}` {
		t.Errorf("unexpected body at server: %q", receivedBody)
	}
}

func TestHandler_Execute_CustomHeader(t *testing.T) {
	var receivedHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeader = r.Header.Get("X-Token")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	h := httprequest.New()
	_, err := h.Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"url":     srv.URL,
			"method":  "GET",
			"headers": map[string]any{"X-Token": "secret"},
		},
		UpstreamData: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedHeader != "secret" {
		t.Errorf("expected X-Token=secret, got %q", receivedHeader)
	}
}

func TestHandler_Execute_NonOKStatusIsNotError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	h := httprequest.New()
	out, err := h.Execute(context.Background(), node.NodeInput{
		Config:       map[string]any{"url": srv.URL, "method": "GET"},
		UpstreamData: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Data["status_code"] != 404 {
		t.Errorf("expected 404, got %v", out.Data["status_code"])
	}
}

func TestHandler_Execute_TemplateURL(t *testing.T) {
	var receivedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	h := httprequest.New()
	_, err := h.Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"url":    srv.URL + "/{{.n1.segment}}",
			"method": "GET",
		},
		UpstreamData: map[string]any{
			"n1": map[string]any{"segment": "users"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedPath != "/users" {
		t.Errorf("expected path=/users, got %q", receivedPath)
	}
}

func TestHandler_Execute_TimeoutSeconds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	h := httprequest.New()
	// timeout_seconds as float64 (JSON unmarshals numbers as float64)
	_, err := h.Execute(context.Background(), node.NodeInput{
		Config:       map[string]any{"url": srv.URL, "method": "GET", "timeout_seconds": float64(30)},
		UpstreamData: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandler_Execute_InvalidTemplate_ReturnsError(t *testing.T) {
	h := httprequest.New()
	_, err := h.Execute(context.Background(), node.NodeInput{
		Config:       map[string]any{"url": "http://example.com/{{.broken", "method": "GET"},
		UpstreamData: map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error for invalid template")
	}
}

func TestHandler_Execute_MissingURL_ReturnsError(t *testing.T) {
	h := httprequest.New()
	_, err := h.Execute(context.Background(), node.NodeInput{
		Config:       map[string]any{"method": "GET"},
		UpstreamData: map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error for missing url")
	}
}
