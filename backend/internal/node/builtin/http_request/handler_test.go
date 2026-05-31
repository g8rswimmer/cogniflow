package httprequest_test

import (
	"context"
	"encoding/json"
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
	json.Unmarshal(h.Meta().InputSchema, &schema)

	if _, ok := schema.Properties["url"]; !ok {
		t.Error("input_schema missing 'url' property")
	}
	if _, ok := schema.Properties["method"]; !ok {
		t.Error("input_schema missing 'method' property")
	}
}

func TestHandler_Execute_ReturnsEmptyOutput(t *testing.T) {
	h := httprequest.New()
	out, err := h.Execute(context.Background(), node.NodeInput{
		Config:       map[string]any{"url": "https://example.com", "method": "GET"},
		UpstreamData: map[string]any{},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Data == nil {
		t.Fatal("expected non-nil Data map")
	}
}

func TestHandler_ImplementsNodeHandler(t *testing.T) {
	var _ node.NodeHandler = httprequest.New()
}
