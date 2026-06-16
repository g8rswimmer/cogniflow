package trigger

import (
	"context"
	"errors"
	"testing"

	"github.com/g8rswimmer/cogniflow/internal/store"
)

// fullMockStore extends mockTriggerStore with ListTriggerConfigs control.
type fullMockStore struct {
	mockTriggerStore
	listConfigs    []store.WorkflowTrigger
	listConfigsErr error
	savedConfig    *store.TriggerConfig
	savedWfID      string
}

func (m *fullMockStore) ListTriggerConfigs(_ context.Context) ([]store.WorkflowTrigger, error) {
	return m.listConfigs, m.listConfigsErr
}

func (m *fullMockStore) SaveTriggerConfig(_ context.Context, wfID string, cfg store.TriggerConfig) error {
	m.savedWfID = wfID
	m.savedConfig = &cfg
	return nil
}
func (m *fullMockStore) SavePluginRegistration(_ context.Context, _ store.PluginRegistration) error {
	return nil
}
func (m *fullMockStore) GetPluginRegistration(_ context.Context, _ string) (store.PluginRegistration, error) {
	return store.PluginRegistration{}, nil
}
func (m *fullMockStore) ListPluginRegistrations(_ context.Context) ([]store.PluginRegistration, error) {
	return nil, nil
}
func (m *fullMockStore) DeletePluginRegistration(_ context.Context, _ string) error { return nil }
func (m *fullMockStore) SaveRunNodeResults(_ context.Context, _ string, _ map[string]store.NodeResult) error {
	return nil
}
func (m *fullMockStore) CreateEvalSuite(_ context.Context, v store.EvalSuite) (store.EvalSuite, error) {
	return v, nil
}
func (m *fullMockStore) GetEvalSuite(_ context.Context, _ string) (store.EvalSuite, error) {
	return store.EvalSuite{}, store.ErrNotFound
}
func (m *fullMockStore) ListEvalSuites(_ context.Context, _ string) ([]store.EvalSuiteSummary, error) {
	return nil, nil
}
func (m *fullMockStore) ListEvalSuitesByCronTrigger(_ context.Context) ([]store.EvalSuite, error) {
	return nil, nil
}
func (m *fullMockStore) UpdateEvalSuite(_ context.Context, v store.EvalSuite) (store.EvalSuite, error) {
	return v, nil
}
func (m *fullMockStore) DeleteEvalSuite(_ context.Context, _ string) error { return nil }
func (m *fullMockStore) CreateTestCase(_ context.Context, v store.TestCase) (store.TestCase, error) {
	return v, nil
}
func (m *fullMockStore) GetTestCase(_ context.Context, _ string) (store.TestCase, error) {
	return store.TestCase{}, store.ErrNotFound
}
func (m *fullMockStore) ListTestCases(_ context.Context, _ string) ([]store.TestCase, error) {
	return nil, nil
}
func (m *fullMockStore) UpdateTestCase(_ context.Context, v store.TestCase) (store.TestCase, error) {
	return v, nil
}
func (m *fullMockStore) DeleteTestCase(_ context.Context, _ string) error { return nil }
func (m *fullMockStore) ReorderTestCases(_ context.Context, _ string, _ []string) error {
	return nil
}
func (m *fullMockStore) CreateEvalRun(_ context.Context, v store.EvalRun) (store.EvalRun, error) {
	return v, nil
}
func (m *fullMockStore) GetEvalRun(_ context.Context, _ string) (store.EvalRun, error) {
	return store.EvalRun{}, store.ErrNotFound
}
func (m *fullMockStore) ListEvalRuns(_ context.Context, _ store.EvalRunFilter) ([]store.EvalRun, error) {
	return nil, nil
}
func (m *fullMockStore) UpdateEvalRunStatus(_ context.Context, _ string, _ store.EvalRunStatus, _ store.EvalRunCounts) error {
	return nil
}
func (m *fullMockStore) IncrementEvalRunCounts(_ context.Context, _ string, _ store.EvalRunCounts) error {
	return nil
}
func (m *fullMockStore) CreateTestCaseResult(_ context.Context, v store.TestCaseResult) (store.TestCaseResult, error) {
	return v, nil
}
func (m *fullMockStore) GetTestCaseResult(_ context.Context, _ string) (store.TestCaseResult, error) {
	return store.TestCaseResult{}, store.ErrNotFound
}
func (m *fullMockStore) ListTestCaseResults(_ context.Context, _ string) ([]store.TestCaseResult, error) {
	return nil, nil
}

// ---- LoadAll tests ----------------------------------------------------------

func TestManager_LoadAll_ActivatesCronTrigger(t *testing.T) {
	ms := &fullMockStore{
		listConfigs: []store.WorkflowTrigger{
			{WorkflowID: "wf-cron", Config: store.TriggerConfig{Kind: "cron", CronExpr: "* * * * *"}},
		},
	}
	disp := &mockDispatcher{}
	m := NewManager(ms, disp)

	if err := m.LoadAll(context.Background()); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if m.cron.entryCount() != 1 {
		t.Errorf("want 1 cron entry after LoadAll, got %d", m.cron.entryCount())
	}
}

func TestManager_LoadAll_SkipsWebhookTrigger(t *testing.T) {
	ms := &fullMockStore{
		listConfigs: []store.WorkflowTrigger{
			{WorkflowID: "wf-hook", Config: store.TriggerConfig{Kind: "webhook"}},
		},
	}
	m := NewManager(ms, &mockDispatcher{})
	if err := m.LoadAll(context.Background()); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if m.cron.entryCount() != 0 {
		t.Errorf("webhook should not add a cron entry; got %d", m.cron.entryCount())
	}
}

func TestManager_LoadAll_StoreError(t *testing.T) {
	ms := &fullMockStore{listConfigsErr: errors.New("db error")}
	m := NewManager(ms, &mockDispatcher{})
	if err := m.LoadAll(context.Background()); err == nil {
		t.Error("expected error when store fails")
	}
}

func TestManager_LoadAll_InvalidCronLogsAndContinues(t *testing.T) {
	ms := &fullMockStore{
		listConfigs: []store.WorkflowTrigger{
			{WorkflowID: "wf-bad", Config: store.TriggerConfig{Kind: "cron", CronExpr: "bad-expr"}},
			{WorkflowID: "wf-ok", Config: store.TriggerConfig{Kind: "cron", CronExpr: "* * * * *"}},
		},
	}
	m := NewManager(ms, &mockDispatcher{})
	// LoadAll should not return an error even when one entry fails.
	if err := m.LoadAll(context.Background()); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	// Only the valid cron entry should be armed.
	if m.cron.entryCount() != 1 {
		t.Errorf("want 1 cron entry (bad one skipped), got %d", m.cron.entryCount())
	}
}

// ---- Upsert tests -----------------------------------------------------------

func TestManager_Upsert_CronAdded(t *testing.T) {
	m := NewManager(&fullMockStore{}, &mockDispatcher{})
	if err := m.Upsert("wf-1", store.TriggerConfig{Kind: "cron", CronExpr: "* * * * *"}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if m.cron.entryCount() != 1 {
		t.Errorf("want 1 cron entry, got %d", m.cron.entryCount())
	}
}

func TestManager_Upsert_ManualDeactivatesExistingCron(t *testing.T) {
	m := NewManager(&fullMockStore{}, &mockDispatcher{})
	_ = m.Upsert("wf-1", store.TriggerConfig{Kind: "cron", CronExpr: "* * * * *"})

	if err := m.Upsert("wf-1", store.TriggerConfig{Kind: "manual"}); err != nil {
		t.Fatalf("Upsert manual: %v", err)
	}
	if m.cron.entryCount() != 0 {
		t.Errorf("want 0 cron entries after switching to manual, got %d", m.cron.entryCount())
	}
}

func TestManager_Upsert_InvalidCronReturnsError(t *testing.T) {
	m := NewManager(&fullMockStore{}, &mockDispatcher{})
	if err := m.Upsert("wf-1", store.TriggerConfig{Kind: "cron", CronExpr: "bad"}); err == nil {
		t.Error("expected error for invalid cron expression")
	}
}

func TestManager_Upsert_WebhookNoOp(t *testing.T) {
	m := NewManager(&fullMockStore{}, &mockDispatcher{})
	if err := m.Upsert("wf-1", store.TriggerConfig{Kind: "webhook"}); err != nil {
		t.Fatalf("Upsert webhook: %v", err)
	}
	if m.cron.entryCount() != 0 {
		t.Errorf("webhook upsert should not arm cron, got %d entries", m.cron.entryCount())
	}
}

func TestManager_Upsert_NilManager(t *testing.T) {
	var m *Manager
	// Must not panic.
	if err := m.Upsert("wf-1", store.TriggerConfig{Kind: "cron"}); err != nil {
		t.Errorf("nil manager Upsert should return nil, got %v", err)
	}
}

// ---- Remove tests -----------------------------------------------------------

func TestManager_Remove_StopsCronJob(t *testing.T) {
	m := NewManager(&fullMockStore{}, &mockDispatcher{})
	_ = m.Upsert("wf-1", store.TriggerConfig{Kind: "cron", CronExpr: "* * * * *"})

	m.Remove("wf-1")
	if m.cron.entryCount() != 0 {
		t.Errorf("want 0 cron entries after Remove, got %d", m.cron.entryCount())
	}
}

func TestManager_Remove_NilManager(t *testing.T) {
	var m *Manager
	m.Remove("wf-1") // must not panic
}
