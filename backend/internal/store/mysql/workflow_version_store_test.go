package mysql

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/g8rswimmer/cogniflow/internal/store"
)

// makeVersionWorkflow returns a Workflow ready for snapshotting. The node
// config uses plain (non-sensitive) values so no vault is needed in unit tests.
func makeVersionWorkflow(id, name string, nodeCount int) store.Workflow {
	nodes := make([]store.WorkflowNode, nodeCount)
	for i := range nodes {
		nodes[i] = store.WorkflowNode{
			ID:     "n" + string(rune('1'+i)),
			TypeID: "http.request",
			Config: map[string]any{"url": "https://example.com"},
		}
	}
	return store.Workflow{
		ID:             id,
		Name:           name,
		TimeoutSeconds: 300,
		Trigger:        store.Trigger{Kind: "manual"},
		Nodes:          nodes,
	}
}

// ---- CreateWorkflowVersion --------------------------------------------------

func TestWorkflowVersionStore_Create_AssignsVersionNumber(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	wf := makeVersionWorkflow("wf-1", "flow", 2)
	insertTestWorkflow(t, s, wf.ID)

	if err := s.CreateWorkflowVersion(ctx, wf); err != nil {
		t.Fatalf("CreateWorkflowVersion (v1): %v", err)
	}
	if err := s.CreateWorkflowVersion(ctx, wf); err != nil {
		t.Fatalf("CreateWorkflowVersion (v2): %v", err)
	}

	summaries, err := s.ListWorkflowVersions(ctx, wf.ID)
	if err != nil {
		t.Fatalf("ListWorkflowVersions: %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("want 2 versions, got %d", len(summaries))
	}
	// List is newest-first.
	if summaries[0].VersionNumber != 2 {
		t.Errorf("want version_number 2 first, got %d", summaries[0].VersionNumber)
	}
	if summaries[1].VersionNumber != 1 {
		t.Errorf("want version_number 1 second, got %d", summaries[1].VersionNumber)
	}
}

func TestWorkflowVersionStore_Create_RecordsNodeCount(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	wf := makeVersionWorkflow("wf-nc", "nodecount", 3)
	insertTestWorkflow(t, s, wf.ID)

	if err := s.CreateWorkflowVersion(ctx, wf); err != nil {
		t.Fatalf("CreateWorkflowVersion: %v", err)
	}

	summaries, err := s.ListWorkflowVersions(ctx, wf.ID)
	if err != nil {
		t.Fatalf("ListWorkflowVersions: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("want 1 version, got %d", len(summaries))
	}
	if summaries[0].NodeCount != 3 {
		t.Errorf("node_count: want 3, got %d", summaries[0].NodeCount)
	}
}

func TestWorkflowVersionStore_Create_PreservesEncryptedBytes(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	ciphertext := []byte("encryptedblob")
	wf := store.Workflow{
		ID:             "wf-enc",
		Name:           "sensitive",
		TimeoutSeconds: 300,
		Trigger:        store.Trigger{Kind: "manual"},
		Nodes: []store.WorkflowNode{
			{
				ID:            "n1",
				TypeID:        "llm.openai",
				Config:        map[string]any{"model": "gpt-4o", "api_key": ciphertext},
				SensitiveKeys: map[string]bool{"api_key": true},
			},
		},
	}
	insertTestWorkflow(t, s, wf.ID)

	if err := s.CreateWorkflowVersion(ctx, wf); err != nil {
		t.Fatalf("CreateWorkflowVersion: %v", err)
	}

	ver, err := s.GetWorkflowVersion(ctx, wf.ID, 1)
	if err != nil {
		t.Fatalf("GetWorkflowVersion: %v", err)
	}
	if len(ver.Definition.Nodes) != 1 {
		t.Fatalf("want 1 node, got %d", len(ver.Definition.Nodes))
	}
	n := ver.Definition.Nodes[0]
	// Sensitive value should come back as []byte (ready for the vault to decrypt).
	gotBytes, ok := n.Config["api_key"].([]byte)
	if !ok {
		t.Fatalf("api_key: want []byte, got %T", n.Config["api_key"])
	}
	if string(gotBytes) != string(ciphertext) {
		t.Errorf("api_key bytes: want %q, got %q", ciphertext, gotBytes)
	}
	if !n.SensitiveKeys["api_key"] {
		t.Error("SensitiveKeys[api_key] should be true")
	}
}

// ---- GetWorkflowVersion -----------------------------------------------------

func TestWorkflowVersionStore_Get_ReturnsDefinition(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	wf := makeVersionWorkflow("wf-get", "gettest", 2)
	insertTestWorkflow(t, s, wf.ID)

	if err := s.CreateWorkflowVersion(ctx, wf); err != nil {
		t.Fatalf("CreateWorkflowVersion: %v", err)
	}

	ver, err := s.GetWorkflowVersion(ctx, wf.ID, 1)
	if err != nil {
		t.Fatalf("GetWorkflowVersion: %v", err)
	}

	if ver.VersionNumber != 1 {
		t.Errorf("version_number: want 1, got %d", ver.VersionNumber)
	}
	if ver.WorkflowID != wf.ID {
		t.Errorf("workflow_id: want %q, got %q", wf.ID, ver.WorkflowID)
	}
	if ver.Definition.Name != wf.Name {
		t.Errorf("definition.name: want %q, got %q", wf.Name, ver.Definition.Name)
	}
	if len(ver.Definition.Nodes) != 2 {
		t.Errorf("definition.nodes: want 2, got %d", len(ver.Definition.Nodes))
	}
}

func TestWorkflowVersionStore_Get_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.GetWorkflowVersion(ctx, "no-such-wf", 1)
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestWorkflowVersionStore_Get_PreservesInitialDataSchema(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	schema := json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}}}`)
	wf := store.Workflow{
		ID:                "wf-schema",
		Name:              "withschema",
		TimeoutSeconds:    300,
		Trigger:           store.Trigger{Kind: "manual"},
		InitialDataSchema: schema,
	}
	insertTestWorkflow(t, s, wf.ID)

	if err := s.CreateWorkflowVersion(ctx, wf); err != nil {
		t.Fatalf("CreateWorkflowVersion: %v", err)
	}

	ver, err := s.GetWorkflowVersion(ctx, wf.ID, 1)
	if err != nil {
		t.Fatalf("GetWorkflowVersion: %v", err)
	}
	if string(ver.Definition.InitialDataSchema) != string(schema) {
		t.Errorf("initial_data_schema: want %s, got %s", schema, ver.Definition.InitialDataSchema)
	}
}

// ---- ListWorkflowVersions ---------------------------------------------------

func TestWorkflowVersionStore_List_EmptyForNewWorkflow(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	insertTestWorkflow(t, s, "wf-empty")

	summaries, err := s.ListWorkflowVersions(ctx, "wf-empty")
	if err != nil {
		t.Fatalf("ListWorkflowVersions: %v", err)
	}
	if len(summaries) != 0 {
		t.Errorf("want 0 versions, got %d", len(summaries))
	}
}

func TestWorkflowVersionStore_List_OrderedNewestFirst(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	wf := makeVersionWorkflow("wf-list", "listtest", 1)
	insertTestWorkflow(t, s, wf.ID)

	for i := 0; i < 3; i++ {
		if err := s.CreateWorkflowVersion(ctx, wf); err != nil {
			t.Fatalf("CreateWorkflowVersion iteration %d: %v", i, err)
		}
	}

	summaries, err := s.ListWorkflowVersions(ctx, wf.ID)
	if err != nil {
		t.Fatalf("ListWorkflowVersions: %v", err)
	}
	if len(summaries) != 3 {
		t.Fatalf("want 3 summaries, got %d", len(summaries))
	}
	for i, s := range summaries {
		wantVer := 3 - i
		if s.VersionNumber != wantVer {
			t.Errorf("summaries[%d].version_number: want %d, got %d", i, wantVer, s.VersionNumber)
		}
	}
}

// ---- DeleteWorkflowVersions -------------------------------------------------

func TestWorkflowVersionStore_Delete_RemovesAll(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	wf := makeVersionWorkflow("wf-del", "delete", 1)
	insertTestWorkflow(t, s, wf.ID)

	for i := 0; i < 3; i++ {
		if err := s.CreateWorkflowVersion(ctx, wf); err != nil {
			t.Fatalf("CreateWorkflowVersion: %v", err)
		}
	}

	if err := s.DeleteWorkflowVersions(ctx, wf.ID); err != nil {
		t.Fatalf("DeleteWorkflowVersions: %v", err)
	}

	summaries, err := s.ListWorkflowVersions(ctx, wf.ID)
	if err != nil {
		t.Fatalf("ListWorkflowVersions after delete: %v", err)
	}
	if len(summaries) != 0 {
		t.Errorf("want 0 versions after delete, got %d", len(summaries))
	}
}

// ---- RestoreWorkflowVersion -------------------------------------------------

func TestWorkflowVersionStore_Restore_ReplacesNodes(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create workflow with 2 nodes and snapshot as version 1.
	wf := makeVersionWorkflow("wf-restore", "restore", 2)
	if _, err := s.CreateWorkflow(ctx, wf); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := s.CreateWorkflowVersion(ctx, wf); err != nil {
		t.Fatalf("CreateWorkflowVersion (v1): %v", err)
	}

	// Snapshot v2 with 1 node (simulates an update that removed a node).
	wf2 := makeVersionWorkflow("wf-restore", "restore-updated", 1)
	if _, err := s.UpdateWorkflow(ctx, wf2); err != nil {
		t.Fatalf("UpdateWorkflow: %v", err)
	}
	if err := s.CreateWorkflowVersion(ctx, wf2); err != nil {
		t.Fatalf("CreateWorkflowVersion (v2): %v", err)
	}

	// Restore to version 1 (2 nodes, original name).
	restored, err := s.RestoreWorkflowVersion(ctx, wf.ID, 1)
	if err != nil {
		t.Fatalf("RestoreWorkflowVersion: %v", err)
	}
	if len(restored.Nodes) != 2 {
		t.Errorf("restored nodes: want 2, got %d", len(restored.Nodes))
	}
	if restored.Name != "restore" {
		t.Errorf("restored name: want %q, got %q", "restore", restored.Name)
	}

	// The restore should have created version 3.
	summaries, err := s.ListWorkflowVersions(ctx, wf.ID)
	if err != nil {
		t.Fatalf("ListWorkflowVersions after restore: %v", err)
	}
	if len(summaries) != 3 {
		t.Errorf("want 3 versions after restore, got %d", len(summaries))
	}
	if summaries[0].VersionNumber != 3 {
		t.Errorf("latest version after restore: want 3, got %d", summaries[0].VersionNumber)
	}
}

func TestWorkflowVersionStore_Restore_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	insertTestWorkflow(t, s, "wf-nf")

	_, err := s.RestoreWorkflowVersion(ctx, "wf-nf", 99)
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestWorkflowVersionStore_Restore_PreservesEncryptedBytes(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	ciphertext := []byte("secret-cipher")
	wf := store.Workflow{
		ID:             "wf-enc-restore",
		Name:           "enc-restore",
		TimeoutSeconds: 300,
		Trigger:        store.Trigger{Kind: "manual"},
		Nodes: []store.WorkflowNode{
			{
				ID:            "n1",
				TypeID:        "llm.openai",
				Config:        map[string]any{"model": "gpt-4o", "api_key": ciphertext},
				SensitiveKeys: map[string]bool{"api_key": true},
			},
		},
	}
	if _, err := s.CreateWorkflow(ctx, wf); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := s.CreateWorkflowVersion(ctx, wf); err != nil {
		t.Fatalf("CreateWorkflowVersion: %v", err)
	}

	// Restore version 1 and verify the ciphertext is intact.
	restored, err := s.RestoreWorkflowVersion(ctx, wf.ID, 1)
	if err != nil {
		t.Fatalf("RestoreWorkflowVersion: %v", err)
	}
	if len(restored.Nodes) != 1 {
		t.Fatalf("want 1 node, got %d", len(restored.Nodes))
	}
	gotBytes, ok := restored.Nodes[0].Config["api_key"].([]byte)
	if !ok {
		t.Fatalf("api_key: want []byte, got %T", restored.Nodes[0].Config["api_key"])
	}
	if string(gotBytes) != string(ciphertext) {
		t.Errorf("api_key: want %q, got %q", ciphertext, gotBytes)
	}
}

// ---- DeleteWorkflow cascades to versions ------------------------------------

func TestWorkflowStore_Delete_CascadesToVersions(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	wf := makeVersionWorkflow("wf-cascade", "cascade", 1)
	if _, err := s.CreateWorkflow(ctx, wf); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := s.CreateWorkflowVersion(ctx, wf); err != nil {
		t.Fatalf("CreateWorkflowVersion: %v", err)
	}

	if err := s.DeleteWorkflow(ctx, wf.ID); err != nil {
		t.Fatalf("DeleteWorkflow: %v", err)
	}

	summaries, err := s.ListWorkflowVersions(ctx, wf.ID)
	if err != nil {
		t.Fatalf("ListWorkflowVersions after DeleteWorkflow: %v", err)
	}
	if len(summaries) != 0 {
		t.Errorf("want 0 versions after DeleteWorkflow, got %d", len(summaries))
	}
}
