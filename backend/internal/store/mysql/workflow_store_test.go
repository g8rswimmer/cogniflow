package mysql

import (
	"context"
	"errors"
	"testing"

	"github.com/g8rswimmer/cogniflow/internal/store"
)

// ---- CreateWorkflow ---------------------------------------------------------

func TestWorkflowStore_Create_MinimalWorkflow(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	in := store.Workflow{Name: "simple"}
	got, err := s.CreateWorkflow(ctx, in)
	if err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}

	if got.ID == "" {
		t.Error("expected a generated ID")
	}
	if got.Name != "simple" {
		t.Errorf("name: want simple, got %q", got.Name)
	}
	if got.TimeoutSeconds != 300 {
		t.Errorf("timeout_seconds: want 300, got %d", got.TimeoutSeconds)
	}
	if got.Trigger.Kind != "manual" {
		t.Errorf("trigger.kind: want manual, got %q", got.Trigger.Kind)
	}
	if got.CreatedAt.IsZero() {
		t.Error("created_at should not be zero")
	}
}

func TestWorkflowStore_Create_WithNodesAndEdges(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	in := store.Workflow{
		ID:   "wf-1",
		Name: "graph",
		Nodes: []store.WorkflowNode{
			{ID: "n1", TypeID: "http.request", Label: "Fetch", Position: store.NodePosition{X: 10, Y: 20}},
			{ID: "n2", TypeID: "http.request", Label: "Post", Position: store.NodePosition{X: 30, Y: 20}},
		},
		Edges: []store.WorkflowEdge{
			{ID: "e1", SourceID: "n1", TargetID: "n2"},
		},
	}
	got, err := s.CreateWorkflow(ctx, in)
	if err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}

	if len(got.Nodes) != 2 {
		t.Fatalf("want 2 nodes, got %d", len(got.Nodes))
	}
	if len(got.Edges) != 1 {
		t.Fatalf("want 1 edge, got %d", len(got.Edges))
	}
	if got.Edges[0].SourceID != "n1" || got.Edges[0].TargetID != "n2" {
		t.Errorf("edge: want n1→n2, got %s→%s", got.Edges[0].SourceID, got.Edges[0].TargetID)
	}
}

func TestWorkflowStore_Create_WithNodeConfig(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	in := store.Workflow{
		ID:   "wf-cfg",
		Name: "configured",
		Nodes: []store.WorkflowNode{
			{
				ID:     "n1",
				TypeID: "http.request",
				Config: map[string]any{"url": "https://example.com", "method": "GET"},
			},
		},
	}
	if _, err := s.CreateWorkflow(ctx, in); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}

	got, err := s.GetWorkflow(ctx, "wf-cfg")
	if err != nil {
		t.Fatalf("GetWorkflow: %v", err)
	}
	if len(got.Nodes) != 1 {
		t.Fatalf("want 1 node, got %d", len(got.Nodes))
	}
	cfg := got.Nodes[0].Config
	if cfg["url"] != "https://example.com" {
		t.Errorf("url: want https://example.com, got %v", cfg["url"])
	}
	if cfg["method"] != "GET" {
		t.Errorf("method: want GET, got %v", cfg["method"])
	}
}

func TestWorkflowStore_Create_WithSensitiveConfig(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	secret := []byte("encrypted-bytes")
	in := store.Workflow{
		ID:   "wf-sec",
		Name: "secure",
		Nodes: []store.WorkflowNode{
			{
				ID:            "n1",
				TypeID:        "http.request",
				Config:        map[string]any{"api_key": secret},
				SensitiveKeys: map[string]bool{"api_key": true},
			},
		},
	}
	if _, err := s.CreateWorkflow(ctx, in); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}

	got, err := s.GetWorkflow(ctx, "wf-sec")
	if err != nil {
		t.Fatalf("GetWorkflow: %v", err)
	}
	raw, ok := got.Nodes[0].Config["api_key"].([]byte)
	if !ok {
		t.Fatalf("expected []byte for sensitive config, got %T", got.Nodes[0].Config["api_key"])
	}
	if string(raw) != "encrypted-bytes" {
		t.Errorf("sensitive value: want %q, got %q", "encrypted-bytes", string(raw))
	}
	if !got.Nodes[0].SensitiveKeys["api_key"] {
		t.Error("api_key should be marked sensitive")
	}
}

func TestWorkflowStore_Create_WithRetryPolicy(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	in := store.Workflow{
		ID:   "wf-retry",
		Name: "retry",
		Nodes: []store.WorkflowNode{
			{
				ID:          "n1",
				TypeID:      "http.request",
				RetryPolicy: &store.RetryPolicy{MaxRetries: 3, BackoffMs: 500},
			},
		},
	}
	if _, err := s.CreateWorkflow(ctx, in); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}

	got, err := s.GetWorkflow(ctx, "wf-retry")
	if err != nil {
		t.Fatalf("GetWorkflow: %v", err)
	}
	rp := got.Nodes[0].RetryPolicy
	if rp == nil {
		t.Fatal("expected retry policy, got nil")
	}
	if rp.MaxRetries != 3 || rp.BackoffMs != 500 {
		t.Errorf("retry policy: want {3, 500}, got %+v", rp)
	}
}

func TestWorkflowStore_Create_WithEdgeBranchLabel(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	label := "true"
	in := store.Workflow{
		ID:   "wf-branch",
		Name: "branching",
		Nodes: []store.WorkflowNode{
			{ID: "n1", TypeID: "conditional"},
			{ID: "n2", TypeID: "http.request"},
		},
		Edges: []store.WorkflowEdge{
			{ID: "e1", SourceID: "n1", TargetID: "n2", BranchLabel: &label},
		},
	}
	if _, err := s.CreateWorkflow(ctx, in); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}

	got, err := s.GetWorkflow(ctx, "wf-branch")
	if err != nil {
		t.Fatalf("GetWorkflow: %v", err)
	}
	if got.Edges[0].BranchLabel == nil || *got.Edges[0].BranchLabel != "true" {
		t.Errorf("branch_label: want %q, got %v", "true", got.Edges[0].BranchLabel)
	}
}

func TestWorkflowStore_Create_CronTrigger(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	in := store.Workflow{
		ID:      "wf-cron",
		Name:    "scheduled",
		Trigger: store.Trigger{Kind: "cron", CronExpr: "0 * * * *"},
	}
	if _, err := s.CreateWorkflow(ctx, in); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}

	got, err := s.GetWorkflow(ctx, "wf-cron")
	if err != nil {
		t.Fatalf("GetWorkflow: %v", err)
	}
	if got.Trigger.Kind != "cron" {
		t.Errorf("trigger.kind: want cron, got %q", got.Trigger.Kind)
	}
	if got.Trigger.CronExpr != "0 * * * *" {
		t.Errorf("trigger.cron_expr: want %q, got %q", "0 * * * *", got.Trigger.CronExpr)
	}
}

// ---- GetWorkflow ------------------------------------------------------------

func TestWorkflowStore_Get_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetWorkflow(context.Background(), "no-such-id")
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestWorkflowStore_Get_RoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	in := store.Workflow{
		ID:             "wf-rt",
		Name:           "round-trip",
		Description:    "desc",
		TimeoutSeconds: 600,
		Trigger:        store.Trigger{Kind: "webhook"},
	}
	created, err := s.CreateWorkflow(ctx, in)
	if err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}

	got, err := s.GetWorkflow(ctx, "wf-rt")
	if err != nil {
		t.Fatalf("GetWorkflow: %v", err)
	}

	if got.Name != in.Name {
		t.Errorf("name: want %q, got %q", in.Name, got.Name)
	}
	if got.Description != in.Description {
		t.Errorf("description: want %q, got %q", in.Description, got.Description)
	}
	if got.TimeoutSeconds != in.TimeoutSeconds {
		t.Errorf("timeout_seconds: want %d, got %d", in.TimeoutSeconds, got.TimeoutSeconds)
	}
	if got.Trigger.Kind != in.Trigger.Kind {
		t.Errorf("trigger.kind: want %q, got %q", in.Trigger.Kind, got.Trigger.Kind)
	}
	withinSecond(t, "created_at", created.CreatedAt, got.CreatedAt)
}

// ---- ListWorkflows ----------------------------------------------------------

func TestWorkflowStore_List_Empty(t *testing.T) {
	s := newTestStore(t)
	got, err := s.ListWorkflows(context.Background())
	if err != nil {
		t.Fatalf("ListWorkflows: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want empty list, got %d", len(got))
	}
}

func TestWorkflowStore_List_ReturnsSummaries(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	for _, name := range []string{"Alpha", "Beta", "Gamma"} {
		if _, err := s.CreateWorkflow(ctx, store.Workflow{Name: name}); err != nil {
			t.Fatalf("CreateWorkflow %q: %v", name, err)
		}
	}

	got, err := s.ListWorkflows(ctx)
	if err != nil {
		t.Fatalf("ListWorkflows: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 summaries, got %d", len(got))
	}
	// Summaries must not include nodes/edges (WorkflowSummary has no such fields).
	for _, s := range got {
		if s.ID == "" || s.Name == "" {
			t.Errorf("summary missing id or name: %+v", s)
		}
	}
}

// ---- UpdateWorkflow ---------------------------------------------------------

func TestWorkflowStore_Update_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.UpdateWorkflow(context.Background(), store.Workflow{ID: "ghost"})
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestWorkflowStore_Update_ChangesName(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	created, err := s.CreateWorkflow(ctx, store.Workflow{ID: "wf-upd", Name: "original"})
	if err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}

	updated, err := s.UpdateWorkflow(ctx, store.Workflow{
		ID:             created.ID,
		Name:           "renamed",
		TimeoutSeconds: 120,
	})
	if err != nil {
		t.Fatalf("UpdateWorkflow: %v", err)
	}
	if updated.Name != "renamed" {
		t.Errorf("name: want renamed, got %q", updated.Name)
	}
	if updated.TimeoutSeconds != 120 {
		t.Errorf("timeout_seconds: want 120, got %d", updated.TimeoutSeconds)
	}
}

func TestWorkflowStore_Update_ReplacesNodes(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create with one node.
	if _, err := s.CreateWorkflow(ctx, store.Workflow{
		ID:    "wf-repl",
		Name:  "before",
		Nodes: []store.WorkflowNode{{ID: "old-n1", TypeID: "http.request"}},
	}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}

	// Update with two completely different nodes.
	if _, err := s.UpdateWorkflow(ctx, store.Workflow{
		ID:   "wf-repl",
		Name: "after",
		Nodes: []store.WorkflowNode{
			{ID: "new-n1", TypeID: "conditional"},
			{ID: "new-n2", TypeID: "http.request"},
		},
	}); err != nil {
		t.Fatalf("UpdateWorkflow: %v", err)
	}

	got, err := s.GetWorkflow(ctx, "wf-repl")
	if err != nil {
		t.Fatalf("GetWorkflow: %v", err)
	}
	if len(got.Nodes) != 2 {
		t.Fatalf("want 2 nodes after update, got %d", len(got.Nodes))
	}
	// Old node must be gone.
	for _, n := range got.Nodes {
		if n.ID == "old-n1" {
			t.Error("old node still present after update")
		}
	}
}

func TestWorkflowStore_Update_ClearsEdgesWhenNoneProvided(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.CreateWorkflow(ctx, store.Workflow{
		ID:    "wf-clr",
		Name:  "before",
		Nodes: []store.WorkflowNode{{ID: "n1", TypeID: "t"}, {ID: "n2", TypeID: "t"}},
		Edges: []store.WorkflowEdge{{ID: "e1", SourceID: "n1", TargetID: "n2"}},
	}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}

	if _, err := s.UpdateWorkflow(ctx, store.Workflow{
		ID:    "wf-clr",
		Name:  "after",
		Nodes: []store.WorkflowNode{{ID: "n1", TypeID: "t"}},
	}); err != nil {
		t.Fatalf("UpdateWorkflow: %v", err)
	}

	got, err := s.GetWorkflow(ctx, "wf-clr")
	if err != nil {
		t.Fatalf("GetWorkflow: %v", err)
	}
	if len(got.Edges) != 0 {
		t.Errorf("want no edges, got %d", len(got.Edges))
	}
}

// ---- DeleteWorkflow ---------------------------------------------------------

func TestWorkflowStore_Delete_Success(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.CreateWorkflow(ctx, store.Workflow{ID: "wf-del", Name: "bye"}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}

	if err := s.DeleteWorkflow(ctx, "wf-del"); err != nil {
		t.Fatalf("DeleteWorkflow: %v", err)
	}

	_, err := s.GetWorkflow(ctx, "wf-del")
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("want ErrNotFound after delete, got %v", err)
	}
}

func TestWorkflowStore_Delete_NotFound(t *testing.T) {
	s := newTestStore(t)
	err := s.DeleteWorkflow(context.Background(), "ghost")
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestWorkflowStore_Delete_CascadesNodesToConfigs(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.CreateWorkflow(ctx, store.Workflow{
		ID:   "wf-cas",
		Name: "cascade",
		Nodes: []store.WorkflowNode{
			{
				ID:     "n1",
				TypeID: "http.request",
				Config: map[string]any{"url": "https://example.com"},
			},
		},
	}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}

	if err := s.DeleteWorkflow(ctx, "wf-cas"); err != nil {
		t.Fatalf("DeleteWorkflow: %v", err)
	}

	// Verify nodes and their configs are gone by checking no rows exist.
	var count int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM workflow_nodes WHERE workflow_id=?`, "wf-cas",
	).Scan(&count); err != nil {
		t.Fatalf("count nodes: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 nodes after cascade delete, got %d", count)
	}

	var cfgCount int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM node_configs WHERE node_id=?`, "n1",
	).Scan(&cfgCount); err != nil {
		t.Fatalf("count configs: %v", err)
	}
	if cfgCount != 0 {
		t.Errorf("expected 0 configs after cascade delete, got %d", cfgCount)
	}
}
