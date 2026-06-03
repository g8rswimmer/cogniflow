package data_transform

import (
	"context"
	"testing"

	"github.com/g8rswimmer/cogniflow/internal/node"
)

func TestHandler_Meta(t *testing.T) {
	h := New()
	meta := h.Meta()
	if meta.TypeID != "data.transform" {
		t.Errorf("want data.transform, got %s", meta.TypeID)
	}
	if meta.Category != "deterministic" {
		t.Errorf("want deterministic, got %s", meta.Category)
	}
}

func TestExecute_LiteralFields(t *testing.T) {
	h := New()
	out, err := h.Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"fields": map[string]any{
				"greeting": "hello",
				"count":    "42",
			},
		},
		UpstreamData: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Data["greeting"] != "hello" {
		t.Errorf("want hello, got %v", out.Data["greeting"])
	}
	if out.Data["count"] != "42" {
		t.Errorf("want 42, got %v", out.Data["count"])
	}
}

func TestExecute_TemplateFields(t *testing.T) {
	h := New()
	out, err := h.Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"fields": map[string]any{
				"message": "Status: {{.n1.status_code}}",
				"body":    "{{.n1.body}}",
			},
		},
		UpstreamData: map[string]any{
			"n1": map[string]any{"status_code": 200, "body": "ok"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Data["message"] != "Status: 200" {
		t.Errorf("want 'Status: 200', got %v", out.Data["message"])
	}
	if out.Data["body"] != "ok" {
		t.Errorf("want 'ok', got %v", out.Data["body"])
	}
}

func TestExecute_MissingFields_ReturnsEmpty(t *testing.T) {
	h := New()
	out, err := h.Execute(context.Background(), node.NodeInput{
		Config:       map[string]any{},
		UpstreamData: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Data) != 0 {
		t.Errorf("want empty output, got %v", out.Data)
	}
}

func TestExecute_InvalidFieldsType_ReturnsError(t *testing.T) {
	h := New()
	_, err := h.Execute(context.Background(), node.NodeInput{
		Config:       map[string]any{"fields": "not-a-map"},
		UpstreamData: map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error for non-map fields")
	}
}

func TestExecute_TemplateMissingKey_ReturnsError(t *testing.T) {
	h := New()
	_, err := h.Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"fields": map[string]any{
				"val": "{{.n1.missing_key}}",
			},
		},
		UpstreamData: map[string]any{
			"n1": map[string]any{"other": "x"},
		},
	})
	if err == nil {
		t.Fatal("expected error for missing template key")
	}
}

func TestExecute_InitialDataInTemplate(t *testing.T) {
	h := New()
	out, err := h.Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"fields": map[string]any{
				"user": "{{._initial.user_id}}",
			},
		},
		UpstreamData: map[string]any{
			"_initial": map[string]any{"user_id": "abc123"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Data["user"] != "abc123" {
		t.Errorf("want abc123, got %v", out.Data["user"])
	}
}

func TestExecute_NonStringFieldValue_PassedThrough(t *testing.T) {
	h := New()
	out, err := h.Execute(context.Background(), node.NodeInput{
		Config: map[string]any{
			"fields": map[string]any{
				"count": 42, // non-string — passed through unchanged
			},
		},
		UpstreamData: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Data["count"] != 42 {
		t.Errorf("want 42 (int), got %v", out.Data["count"])
	}
}
