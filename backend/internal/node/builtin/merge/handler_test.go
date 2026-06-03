package merge

import (
	"context"
	"testing"

	"github.com/g8rswimmer/cogniflow/internal/node"
)

func TestHandler_Meta(t *testing.T) {
	h := New()
	meta := h.Meta()
	if meta.TypeID != "merge" {
		t.Errorf("want merge, got %s", meta.TypeID)
	}
	if meta.Category != "control" {
		t.Errorf("want control, got %s", meta.Category)
	}
}

func TestExecute_MergesTwoUpstreams(t *testing.T) {
	h := New()
	out, err := h.Execute(context.Background(), node.NodeInput{
		Config: map[string]any{},
		UpstreamData: map[string]any{
			"n1": map[string]any{"status_code": 200, "body": "hello"},
			"n2": map[string]any{"completion": "world"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Data["status_code"] != 200 {
		t.Errorf("want status_code 200, got %v", out.Data["status_code"])
	}
	if out.Data["body"] != "hello" {
		t.Errorf("want body hello, got %v", out.Data["body"])
	}
	if out.Data["completion"] != "world" {
		t.Errorf("want completion world, got %v", out.Data["completion"])
	}
}

func TestExecute_ExcludesInitialData(t *testing.T) {
	h := New()
	out, err := h.Execute(context.Background(), node.NodeInput{
		Config: map[string]any{},
		UpstreamData: map[string]any{
			"_initial": map[string]any{"user_id": "42"},
			"n1":       map[string]any{"result": "ok"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := out.Data["user_id"]; ok {
		t.Error("_initial keys should not appear in merge output")
	}
	if out.Data["result"] != "ok" {
		t.Errorf("want result ok, got %v", out.Data["result"])
	}
}

func TestExecute_EmptyUpstream(t *testing.T) {
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

func TestExecute_KeyConflict_LastWriterWins(t *testing.T) {
	// Iteration order over maps is not guaranteed, so we just verify one key wins.
	h := New()
	out, err := h.Execute(context.Background(), node.NodeInput{
		Config: map[string]any{},
		UpstreamData: map[string]any{
			"n1": map[string]any{"shared": "from-n1"},
			"n2": map[string]any{"shared": "from-n2"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	v, ok := out.Data["shared"]
	if !ok {
		t.Fatal("shared key missing from merge output")
	}
	if v != "from-n1" && v != "from-n2" {
		t.Errorf("shared should be from one of the upstreams, got %v", v)
	}
}

func TestExecute_NonMapUpstreamValue_Ignored(t *testing.T) {
	h := New()
	// If an upstream value is not a map (shouldn't happen in practice), it is skipped.
	out, err := h.Execute(context.Background(), node.NodeInput{
		Config: map[string]any{},
		UpstreamData: map[string]any{
			"n1": "unexpected-scalar",
			"n2": map[string]any{"key": "val"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Data["key"] != "val" {
		t.Errorf("want key=val, got %v", out.Data["key"])
	}
}
