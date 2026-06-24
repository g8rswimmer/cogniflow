package crypto

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/g8rswimmer/cogniflow/internal/node"
	"github.com/g8rswimmer/cogniflow/internal/store"
)

// ---- stub infrastructure -------------------------------------------------

type vaultStubStore struct {
	workflow store.Workflow
}

func (s *vaultStubStore) CreateWorkflow(_ context.Context, w store.Workflow) (store.Workflow, error) {
	s.workflow = w
	return w, nil
}
func (s *vaultStubStore) GetWorkflow(_ context.Context, _ string) (store.Workflow, error) {
	return s.workflow, nil
}
func (s *vaultStubStore) ListWorkflows(_ context.Context) ([]store.WorkflowSummary, error) {
	return nil, nil
}
func (s *vaultStubStore) UpdateWorkflow(_ context.Context, w store.Workflow) (store.Workflow, error) {
	s.workflow = w
	return w, nil
}
func (s *vaultStubStore) DeleteWorkflow(_ context.Context, _ string) error { return nil }
func (s *vaultStubStore) GetWorkflowSchema(_ context.Context, _ string) (json.RawMessage, error) {
	return s.workflow.InitialDataSchema, nil
}

func (s *vaultStubStore) CreateRun(_ context.Context, r store.Run) (store.Run, error) { return r, nil }
func (s *vaultStubStore) UpdateRunStatus(_ context.Context, _ string, _ store.RunStatus, _ map[string]any) error {
	return nil
}
func (s *vaultStubStore) SaveRunNodeResults(_ context.Context, _ string, _ map[string]store.NodeResult) error {
	return nil
}
func (s *vaultStubStore) GetRun(_ context.Context, _ string) (store.Run, error) { return store.Run{}, nil }
func (s *vaultStubStore) ListRuns(_ context.Context, _ store.RunFilter) ([]store.Run, error) {
	return nil, nil
}
func (s *vaultStubStore) UpsertChunks(_ context.Context, _ []store.RAGChunk) error { return nil }
func (s *vaultStubStore) SearchChunks(_ context.Context, _ []float32, _ int, _ string) ([]store.RAGChunkResult, error) {
	return nil, nil
}
func (s *vaultStubStore) SaveTriggerConfig(_ context.Context, _ string, _ store.TriggerConfig) error {
	return nil
}
func (s *vaultStubStore) GetTriggerConfig(_ context.Context, _ string) (store.TriggerConfig, error) {
	return store.TriggerConfig{}, nil
}
func (s *vaultStubStore) ListTriggerConfigs(_ context.Context) ([]store.WorkflowTrigger, error) {
	return nil, nil
}
func (s *vaultStubStore) SavePluginRegistration(_ context.Context, _ store.PluginRegistration) error {
	return nil
}
func (s *vaultStubStore) GetPluginRegistration(_ context.Context, _ string) (store.PluginRegistration, error) {
	return store.PluginRegistration{}, nil
}
func (s *vaultStubStore) ListPluginRegistrations(_ context.Context) ([]store.PluginRegistration, error) {
	return nil, nil
}
func (s *vaultStubStore) DeletePluginRegistration(_ context.Context, _ string) error { return nil }
func (s *vaultStubStore) SaveGraderRegistration(_ context.Context, _ store.GraderRegistration) error {
	return nil
}
func (s *vaultStubStore) GetGraderRegistration(_ context.Context, _ string) (store.GraderRegistration, error) {
	return store.GraderRegistration{}, store.ErrNotFound
}
func (s *vaultStubStore) ListGraderRegistrations(_ context.Context) ([]store.GraderRegistration, error) {
	return nil, nil
}
func (s *vaultStubStore) DeleteGraderRegistration(_ context.Context, _ string) error { return nil }
func (s *vaultStubStore) CreateEvalSuite(_ context.Context, v store.EvalSuite) (store.EvalSuite, error) {
	return v, nil
}
func (s *vaultStubStore) GetEvalSuite(_ context.Context, _ string) (store.EvalSuite, error) {
	return store.EvalSuite{}, store.ErrNotFound
}
func (s *vaultStubStore) ListEvalSuites(_ context.Context, _ string) ([]store.EvalSuiteSummary, error) {
	return nil, nil
}
func (s *vaultStubStore) ListEvalSuitesByCronTrigger(_ context.Context) ([]store.EvalSuite, error) {
	return nil, nil
}
func (s *vaultStubStore) UpdateEvalSuite(_ context.Context, v store.EvalSuite) (store.EvalSuite, error) {
	return v, nil
}
func (s *vaultStubStore) DeleteEvalSuite(_ context.Context, _ string) error { return nil }
func (s *vaultStubStore) CreateTestCase(_ context.Context, v store.TestCase) (store.TestCase, error) {
	return v, nil
}
func (s *vaultStubStore) GetTestCase(_ context.Context, _ string) (store.TestCase, error) {
	return store.TestCase{}, store.ErrNotFound
}
func (s *vaultStubStore) ListTestCases(_ context.Context, _ string) ([]store.TestCase, error) {
	return nil, nil
}
func (s *vaultStubStore) UpdateTestCase(_ context.Context, v store.TestCase) (store.TestCase, error) {
	return v, nil
}
func (s *vaultStubStore) DeleteTestCase(_ context.Context, _ string) error { return nil }
func (s *vaultStubStore) ReorderTestCases(_ context.Context, _ string, _ []string) error {
	return nil
}
func (s *vaultStubStore) CreateEvalRun(_ context.Context, v store.EvalRun) (store.EvalRun, error) {
	return v, nil
}
func (s *vaultStubStore) GetEvalRun(_ context.Context, _ string) (store.EvalRun, error) {
	return store.EvalRun{}, store.ErrNotFound
}
func (s *vaultStubStore) ListEvalRuns(_ context.Context, _ store.EvalRunFilter) ([]store.EvalRun, error) {
	return nil, nil
}
func (s *vaultStubStore) UpdateEvalRunStatus(_ context.Context, _ string, _ store.EvalRunStatus, _ store.EvalRunCounts) error {
	return nil
}
func (s *vaultStubStore) IncrementEvalRunCounts(_ context.Context, _ string, _ store.EvalRunCounts) error {
	return nil
}
func (s *vaultStubStore) CreateTestCaseResult(_ context.Context, v store.TestCaseResult) (store.TestCaseResult, error) {
	return v, nil
}
func (s *vaultStubStore) GetTestCaseResult(_ context.Context, _ string) (store.TestCaseResult, error) {
	return store.TestCaseResult{}, store.ErrNotFound
}
func (s *vaultStubStore) ListTestCaseResults(_ context.Context, _ string) ([]store.TestCaseResult, error) {
	return nil, nil
}
func (s *vaultStubStore) CreateWorkflowVersion(_ context.Context, _ store.Workflow) error { return nil }
func (s *vaultStubStore) GetLatestWorkflowVersionNumber(_ context.Context, _ string) (*int, error) {
	return nil, nil
}
func (s *vaultStubStore) ListWorkflowVersions(_ context.Context, _ string) ([]store.WorkflowVersionSummary, error) {
	return nil, nil
}
func (s *vaultStubStore) GetWorkflowVersion(_ context.Context, _ string, _ int) (store.WorkflowVersion, error) {
	return store.WorkflowVersion{}, store.ErrNotFound
}
func (s *vaultStubStore) DeleteWorkflowVersions(_ context.Context, _ string) error { return nil }
func (s *vaultStubStore) RestoreWorkflowVersion(_ context.Context, _ string, _ int) (store.Workflow, error) {
	return store.Workflow{}, store.ErrNotFound
}

// Auth stubs — not used by vault tests; required to implement store.Store.
func (s *vaultStubStore) CreateOrganization(_ context.Context, org store.Organization) (store.Organization, error) {
	return org, nil
}
func (s *vaultStubStore) GetOrganization(_ context.Context, _ string) (store.Organization, error) {
	return store.Organization{}, store.ErrNotFound
}
func (s *vaultStubStore) ListOrganizations(_ context.Context) ([]store.Organization, error) {
	return nil, nil
}
func (s *vaultStubStore) DeleteOrganization(_ context.Context, _ string) error { return nil }
func (s *vaultStubStore) CreateUser(_ context.Context, u store.User) (store.User, error) {
	return u, nil
}
func (s *vaultStubStore) GetUser(_ context.Context, _ string) (store.User, error) {
	return store.User{}, store.ErrNotFound
}
func (s *vaultStubStore) GetUserByEmail(_ context.Context, _ string) (store.User, error) {
	return store.User{}, store.ErrNotFound
}
func (s *vaultStubStore) ListUsers(_ context.Context, _ string) ([]store.User, error) {
	return nil, nil
}
func (s *vaultStubStore) UpdateUserRole(_ context.Context, _, _ string) error { return nil }
func (s *vaultStubStore) UpdateUserPermissions(_ context.Context, _ string, _ []string) error {
	return nil
}
func (s *vaultStubStore) DeleteUser(_ context.Context, _ string) error { return nil }
func (s *vaultStubStore) CreateInvitation(_ context.Context, inv store.Invitation) (store.Invitation, error) {
	return inv, nil
}
func (s *vaultStubStore) GetInvitationByToken(_ context.Context, _ string) (store.Invitation, error) {
	return store.Invitation{}, store.ErrNotFound
}
func (s *vaultStubStore) AcceptInvitation(_ context.Context, _ string, _ time.Time) error {
	return nil
}

// stubNode has one sensitive field "api_key".
type stubNode struct{}

var sensitiveSchema = json.RawMessage(`{
  "type":"object",
  "properties":{
    "api_key":  {"type":"string","x-sensitive":true},
    "model":    {"type":"string"}
  }
}`)

func (n *stubNode) Meta() node.NodeMeta {
	return node.NodeMeta{
		TypeID:       "ai.stub",
		DisplayName:  "Stub AI",
		Category:     "ai",
		InputSchema:  sensitiveSchema,
		OutputSchema: json.RawMessage(`{}`),
	}
}
func (n *stubNode) Execute(_ context.Context, _ node.NodeInput) (node.NodeOutput, error) {
	return node.NodeOutput{}, nil
}

func newTestVault(t *testing.T) (*ConfigVault, *vaultStubStore) {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 42)
	}
	cipher, err := NewCipher(key)
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}
	registry := node.NewRegistry()
	registry.Register(&stubNode{})

	inner := &vaultStubStore{}
	vault := NewConfigVault(inner, cipher, registry)
	return vault, inner
}

// ---- tests ---------------------------------------------------------------

func TestConfigVault_CreateWorkflow_EncryptsSensitiveField(t *testing.T) {
	vault, inner := newTestVault(t)

	wf := store.Workflow{
		ID:   "wf-1",
		Name: "Test",
		Nodes: []store.WorkflowNode{
			{
				ID:     "n1",
				TypeID: "ai.stub",
				Config: map[string]any{"api_key": "sk-secret", "model": "gpt-4"},
			},
		},
	}

	_, err := vault.CreateWorkflow(context.Background(), wf)
	if err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}

	// The inner store should have received encrypted bytes for api_key.
	stored := inner.workflow.Nodes[0]
	apiKeyVal := stored.Config["api_key"]
	if _, ok := apiKeyVal.([]byte); !ok {
		t.Fatalf("expected []byte ciphertext for api_key, got %T (%v)", apiKeyVal, apiKeyVal)
	}
	if !stored.SensitiveKeys["api_key"] {
		t.Fatal("SensitiveKeys[api_key] should be true")
	}

	// Non-sensitive field is stored as-is.
	if stored.Config["model"] != "gpt-4" {
		t.Fatalf("expected model='gpt-4', got %v", stored.Config["model"])
	}
	if stored.SensitiveKeys["model"] {
		t.Fatal("SensitiveKeys[model] should be false")
	}
}

func TestConfigVault_GetWorkflow_DecryptsSensitiveField(t *testing.T) {
	vault, _ := newTestVault(t)

	// Create first (which encrypts).
	wf := store.Workflow{
		ID:   "wf-2",
		Name: "Decrypt Test",
		Nodes: []store.WorkflowNode{
			{
				ID:     "n1",
				TypeID: "ai.stub",
				Config: map[string]any{"api_key": "sk-super-secret", "model": "gpt-4"},
			},
		},
	}
	_, err := vault.CreateWorkflow(context.Background(), wf)
	if err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}

	// GetWorkflow decrypts.
	got, err := vault.GetWorkflow(context.Background(), "wf-2")
	if err != nil {
		t.Fatalf("GetWorkflow: %v", err)
	}

	apiKey, ok := got.Nodes[0].Config["api_key"].(string)
	if !ok {
		t.Fatalf("expected string for decrypted api_key, got %T", got.Nodes[0].Config["api_key"])
	}
	if apiKey != "sk-super-secret" {
		t.Fatalf("expected 'sk-super-secret', got %q", apiKey)
	}

	// SensitiveKeys is preserved so the API handler can mask it.
	if !got.Nodes[0].SensitiveKeys["api_key"] {
		t.Fatal("SensitiveKeys[api_key] should remain true after decryption")
	}
}

func TestConfigVault_NoSensitiveFields_PassThrough(t *testing.T) {
	vault, inner := newTestVault(t)

	wf := store.Workflow{
		ID:   "wf-3",
		Name: "No Sensitive",
		Nodes: []store.WorkflowNode{
			{
				ID:     "n1",
				TypeID: "ai.stub",
				Config: map[string]any{"model": "gpt-3.5-turbo"},
			},
		},
	}
	_, err := vault.CreateWorkflow(context.Background(), wf)
	if err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}

	// model should NOT be encrypted.
	stored := inner.workflow.Nodes[0]
	if _, ok := stored.Config["model"].([]byte); ok {
		t.Fatal("non-sensitive field should not be encrypted")
	}
	if stored.SensitiveKeys["model"] {
		t.Fatal("model should not be in SensitiveKeys")
	}
}

func TestConfigVault_UnknownNodeType_SkipsEncryption(t *testing.T) {
	vault, inner := newTestVault(t)

	wf := store.Workflow{
		ID:   "wf-4",
		Name: "Unknown Node",
		Nodes: []store.WorkflowNode{
			{
				ID:     "n1",
				TypeID: "unknown.type",
				Config: map[string]any{"api_key": "should-not-encrypt"},
			},
		},
	}
	_, err := vault.CreateWorkflow(context.Background(), wf)
	if err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}

	// Unknown type: no encryption.
	stored := inner.workflow.Nodes[0]
	if _, ok := stored.Config["api_key"].([]byte); ok {
		t.Fatal("unknown type: api_key should not be encrypted")
	}
}

func TestConfigVault_RoundTrip_MultipleNodes(t *testing.T) {
	vault, _ := newTestVault(t)

	wf := store.Workflow{
		ID:   "wf-5",
		Name: "Multi",
		Nodes: []store.WorkflowNode{
			{ID: "n1", TypeID: "ai.stub", Config: map[string]any{"api_key": "key1", "model": "m1"}},
			{ID: "n2", TypeID: "ai.stub", Config: map[string]any{"api_key": "key2", "model": "m2"}},
		},
	}

	_, err := vault.CreateWorkflow(context.Background(), wf)
	if err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}

	got, err := vault.GetWorkflow(context.Background(), "wf-5")
	if err != nil {
		t.Fatalf("GetWorkflow: %v", err)
	}

	for i, expected := range []string{"key1", "key2"} {
		k, ok := got.Nodes[i].Config["api_key"].(string)
		if !ok || k != expected {
			t.Fatalf("node %d: expected api_key=%q, got %v", i, expected, got.Nodes[i].Config["api_key"])
		}
	}
}

func TestConfigVault_ListWorkflows_Delegates(t *testing.T) {
	vault, _ := newTestVault(t)
	summaries, err := vault.ListWorkflows(context.Background())
	if err != nil {
		t.Fatalf("ListWorkflows: %v", err)
	}
	if summaries != nil {
		t.Fatalf("expected nil from stub, got %v", summaries)
	}
}

func TestConfigVault_DeleteWorkflow_Delegates(t *testing.T) {
	vault, _ := newTestVault(t)
	if err := vault.DeleteWorkflow(context.Background(), "any-id"); err != nil {
		t.Fatalf("DeleteWorkflow: %v", err)
	}
}

func TestConfigVault_UpdateWorkflow_EncryptsAndDecrypts(t *testing.T) {
	vault, inner := newTestVault(t)

	wf := store.Workflow{
		ID:   "wf-upd",
		Name: "Update Test",
		Nodes: []store.WorkflowNode{
			{ID: "n1", TypeID: "ai.stub", Config: map[string]any{"api_key": "original-key", "model": "m1"}},
		},
	}
	_, err := vault.UpdateWorkflow(context.Background(), wf)
	if err != nil {
		t.Fatalf("UpdateWorkflow: %v", err)
	}

	// Inner store should have encrypted ciphertext.
	stored := inner.workflow.Nodes[0]
	if _, ok := stored.Config["api_key"].([]byte); !ok {
		t.Fatalf("api_key should be encrypted []byte, got %T", stored.Config["api_key"])
	}
}

func TestConfigVault_CreateRun_Delegates(t *testing.T) {
	vault, _ := newTestVault(t)
	run := store.Run{ID: "run-1", WorkflowID: "wf-1", TriggeredBy: "manual", Status: store.RunStatusPending}
	got, err := vault.CreateRun(context.Background(), run)
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if got.ID != "run-1" {
		t.Fatalf("expected run-1, got %s", got.ID)
	}
}

func TestConfigVault_UpdateRunStatus_Delegates(t *testing.T) {
	vault, _ := newTestVault(t)
	if err := vault.UpdateRunStatus(context.Background(), "run-1", store.RunStatusSucceeded, nil); err != nil {
		t.Fatalf("UpdateRunStatus: %v", err)
	}
}

func TestConfigVault_GetRun_Delegates(t *testing.T) {
	vault, _ := newTestVault(t)
	_, err := vault.GetRun(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
}

func TestConfigVault_ListRuns_Delegates(t *testing.T) {
	vault, _ := newTestVault(t)
	_, err := vault.ListRuns(context.Background(), store.RunFilter{})
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
}

func TestConfigVault_UpsertChunks_Delegates(t *testing.T) {
	vault, _ := newTestVault(t)
	if err := vault.UpsertChunks(context.Background(), nil); err != nil {
		t.Fatalf("UpsertChunks: %v", err)
	}
}

func TestConfigVault_SearchChunks_Delegates(t *testing.T) {
	vault, _ := newTestVault(t)
	_, err := vault.SearchChunks(context.Background(), nil, 5, "")
	if err != nil {
		t.Fatalf("SearchChunks: %v", err)
	}
}

func TestConfigVault_SaveTriggerConfig_Delegates(t *testing.T) {
	vault, _ := newTestVault(t)
	if err := vault.SaveTriggerConfig(context.Background(), "wf-1", store.TriggerConfig{}); err != nil {
		t.Fatalf("SaveTriggerConfig: %v", err)
	}
}

func TestConfigVault_GetTriggerConfig_Delegates(t *testing.T) {
	vault, _ := newTestVault(t)
	_, err := vault.GetTriggerConfig(context.Background(), "wf-1")
	if err != nil {
		t.Fatalf("GetTriggerConfig: %v", err)
	}
}

func TestConfigVault_ListTriggerConfigs_Delegates(t *testing.T) {
	vault, _ := newTestVault(t)
	_, err := vault.ListTriggerConfigs(context.Background())
	if err != nil {
		t.Fatalf("ListTriggerConfigs: %v", err)
	}
}

func TestConfigVault_GetWorkflow_DecryptHandlesNonByteValue(t *testing.T) {
	// If a sensitive value was somehow stored as a non-[]byte, decryptNodes should skip it.
	vault, inner := newTestVault(t)

	// Store a workflow directly in inner with a non-[]byte sensitive value.
	inner.workflow = store.Workflow{
		ID:   "wf-bad",
		Name: "Bad Sensitive",
		Nodes: []store.WorkflowNode{
			{
				ID:            "n1",
				TypeID:        "ai.stub",
				Config:        map[string]any{"api_key": "not-encrypted-bytes"},
				SensitiveKeys: map[string]bool{"api_key": true},
			},
		},
	}

	// GetWorkflow should not panic; the non-[]byte sensitive value is left as-is.
	got, err := vault.GetWorkflow(context.Background(), "wf-bad")
	if err != nil {
		t.Fatalf("GetWorkflow: %v", err)
	}
	// The value is not []byte so decryptNodes skips it.
	_ = got
}

func TestParseSensitiveKeys_IdentifiesXSensitive(t *testing.T) {
	schema := json.RawMessage(`{
		"type":"object",
		"properties":{
			"url":     {"type":"string"},
			"api_key": {"type":"string","x-sensitive":true},
			"token":   {"type":"string","x-sensitive":true}
		}
	}`)
	keys := parseSensitiveKeys(schema)
	if len(keys) != 2 {
		t.Fatalf("expected 2 sensitive keys, got %d (%v)", len(keys), keys)
	}
	keySet := map[string]bool{}
	for _, k := range keys {
		keySet[k] = true
	}
	if !keySet["api_key"] || !keySet["token"] {
		t.Fatalf("expected api_key and token, got %v", keys)
	}
}

func TestParseSensitiveKeys_EmptySchema(t *testing.T) {
	keys := parseSensitiveKeys(json.RawMessage(`{}`))
	if len(keys) != 0 {
		t.Fatalf("expected empty, got %v", keys)
	}
}

func TestParseSensitiveKeys_InvalidJSON(t *testing.T) {
	keys := parseSensitiveKeys(json.RawMessage(`{bad}`))
	if keys != nil {
		t.Fatalf("expected nil for invalid JSON, got %v", keys)
	}
}
