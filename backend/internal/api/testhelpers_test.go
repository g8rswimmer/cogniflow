package api

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/g8rswimmer/cogniflow/internal/store"
	"github.com/g8rswimmer/cogniflow/internal/trigger"
)

// mockStore is an in-memory store.Store implementation for handler tests.
type mockStore struct {
	mu            sync.RWMutex
	workflows     map[string]store.Workflow
	runs          map[string]store.Run
	plugins       map[string]store.PluginRegistration
	graderPlugins map[string]store.GraderRegistration

	// Per-method error overrides.
	createErr        error
	listErr          error
	getErr           error
	updateErr        error
	deleteErr        error
	createRunErr     error
	getRunErr        error
	savePluginErr    error
	listPluginsErr   error
	deletePluginErr  error
}

func newMockStore() *mockStore {
	return &mockStore{
		workflows:     make(map[string]store.Workflow),
		runs:          make(map[string]store.Run),
		plugins:       make(map[string]store.PluginRegistration),
		graderPlugins: make(map[string]store.GraderRegistration),
	}
}

var errInternal = errors.New("simulated internal error")

func (m *mockStore) CreateWorkflow(_ context.Context, w store.Workflow) (store.Workflow, error) {
	if m.createErr != nil {
		return store.Workflow{}, m.createErr
	}
	if w.ID == "" {
		w.ID = "generated-id"
	}
	m.mu.Lock()
	m.workflows[w.ID] = w
	m.mu.Unlock()
	return w, nil
}

func (m *mockStore) GetWorkflow(_ context.Context, id string) (store.Workflow, error) {
	if m.getErr != nil {
		return store.Workflow{}, m.getErr
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	w, ok := m.workflows[id]
	if !ok {
		return store.Workflow{}, store.ErrNotFound
	}
	return w, nil
}

func (m *mockStore) GetWorkflowSchema(_ context.Context, id string) (json.RawMessage, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	w, ok := m.workflows[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	return w.InitialDataSchema, nil
}

func (m *mockStore) ListWorkflows(_ context.Context) ([]store.WorkflowSummary, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	summaries := make([]store.WorkflowSummary, 0, len(m.workflows))
	for _, w := range m.workflows {
		summaries = append(summaries, store.WorkflowSummary{
			ID:          w.ID,
			Name:        w.Name,
			Description: w.Description,
			TriggerKind: w.Trigger.Kind,
		})
	}
	return summaries, nil
}

func (m *mockStore) UpdateWorkflow(_ context.Context, w store.Workflow) (store.Workflow, error) {
	if m.updateErr != nil {
		return store.Workflow{}, m.updateErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.workflows[w.ID]; !ok {
		return store.Workflow{}, store.ErrNotFound
	}
	m.workflows[w.ID] = w
	return w, nil
}

func (m *mockStore) DeleteWorkflow(_ context.Context, id string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.workflows[id]; !ok {
		return store.ErrNotFound
	}
	delete(m.workflows, id)
	return nil
}

// Run store methods — used by run_handler tests.

func (m *mockStore) CreateRun(_ context.Context, r store.Run) (store.Run, error) {
	if m.createRunErr != nil {
		return store.Run{}, m.createRunErr
	}
	if r.ID == "" {
		r.ID = "run-generated-id"
	}
	m.mu.Lock()
	m.runs[r.ID] = r
	m.mu.Unlock()
	return r, nil
}
func (m *mockStore) SaveRunNodeResults(_ context.Context, _ string, _ map[string]store.NodeResult) error {
	return nil
}
func (m *mockStore) UpdateRunStatus(_ context.Context, runID string, status store.RunStatus, _ map[string]any) error {
	m.mu.Lock()
	r := m.runs[runID]
	r.Status = status
	m.runs[runID] = r
	m.mu.Unlock()
	return nil
}
func (m *mockStore) GetRun(_ context.Context, id string) (store.Run, error) {
	if m.getRunErr != nil {
		return store.Run{}, m.getRunErr
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.runs[id]
	if !ok {
		return store.Run{}, store.ErrNotFound
	}
	return r, nil
}
func (m *mockStore) ListRuns(_ context.Context, f store.RunFilter) ([]store.Run, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []store.Run
	for _, r := range m.runs {
		if f.WorkflowID != "" && r.WorkflowID != f.WorkflowID {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

// mockDispatcher implements trigger.Dispatcher for tests.
type mockDispatcher struct {
	runID   string
	dispErr error
}

func (d *mockDispatcher) Dispatch(_ context.Context, _ trigger.RunRequest) (string, error) {
	return d.runID, d.dispErr
}
func (m *mockStore) UpsertChunks(_ context.Context, _ []store.RAGChunk) error { return nil }
func (m *mockStore) SearchChunks(_ context.Context, _ []float32, _ int, _ string) ([]store.RAGChunkResult, error) {
	return nil, nil
}
func (m *mockStore) SaveTriggerConfig(_ context.Context, _ string, _ store.TriggerConfig) error {
	return nil
}
func (m *mockStore) GetTriggerConfig(_ context.Context, _ string) (store.TriggerConfig, error) {
	return store.TriggerConfig{}, nil
}
func (m *mockStore) ListTriggerConfigs(_ context.Context) ([]store.WorkflowTrigger, error) {
	return nil, nil
}

func (m *mockStore) SavePluginRegistration(_ context.Context, reg store.PluginRegistration) error {
	if m.savePluginErr != nil {
		return m.savePluginErr
	}
	m.mu.Lock()
	m.plugins[reg.TypeID] = reg
	m.mu.Unlock()
	return nil
}
func (m *mockStore) GetPluginRegistration(_ context.Context, typeID string) (store.PluginRegistration, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	reg, ok := m.plugins[typeID]
	if !ok {
		return store.PluginRegistration{}, store.ErrNotFound
	}
	return reg, nil
}
func (m *mockStore) ListPluginRegistrations(_ context.Context) ([]store.PluginRegistration, error) {
	if m.listPluginsErr != nil {
		return nil, m.listPluginsErr
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	regs := make([]store.PluginRegistration, 0, len(m.plugins))
	for _, r := range m.plugins {
		regs = append(regs, r)
	}
	return regs, nil
}
func (m *mockStore) DeletePluginRegistration(_ context.Context, typeID string) error {
	if m.deletePluginErr != nil {
		return m.deletePluginErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.plugins[typeID]; !ok {
		return store.ErrNotFound
	}
	delete(m.plugins, typeID)
	return nil
}

// Grader plugin registration methods — in-memory implementations.
func (m *mockStore) SaveGraderRegistration(_ context.Context, reg store.GraderRegistration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.graderPlugins[reg.TypeID] = reg
	return nil
}
func (m *mockStore) GetGraderRegistration(_ context.Context, typeID string) (store.GraderRegistration, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.graderPlugins[typeID]
	if !ok {
		return store.GraderRegistration{}, store.ErrNotFound
	}
	return r, nil
}
func (m *mockStore) ListGraderRegistrations(_ context.Context) ([]store.GraderRegistration, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	regs := make([]store.GraderRegistration, 0, len(m.graderPlugins))
	for _, r := range m.graderPlugins {
		regs = append(regs, r)
	}
	return regs, nil
}
func (m *mockStore) DeleteGraderRegistration(_ context.Context, typeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.graderPlugins[typeID]; !ok {
		return store.ErrNotFound
	}
	delete(m.graderPlugins, typeID)
	return nil
}

// Eval store methods — stubbed; not exercised by api handler tests.

func (m *mockStore) CreateEvalSuite(_ context.Context, s store.EvalSuite) (store.EvalSuite, error) {
	return s, nil
}
func (m *mockStore) GetEvalSuite(_ context.Context, _ string) (store.EvalSuite, error) {
	return store.EvalSuite{}, store.ErrNotFound
}
func (m *mockStore) ListEvalSuites(_ context.Context, _ string) ([]store.EvalSuiteSummary, error) {
	return nil, nil
}
func (m *mockStore) ListEvalSuitesByCronTrigger(_ context.Context) ([]store.EvalSuite, error) {
	return nil, nil
}
func (m *mockStore) UpdateEvalSuite(_ context.Context, s store.EvalSuite) (store.EvalSuite, error) {
	return s, nil
}
func (m *mockStore) DeleteEvalSuite(_ context.Context, _ string) error { return nil }

func (m *mockStore) CreateTestCase(_ context.Context, tc store.TestCase) (store.TestCase, error) {
	return tc, nil
}
func (m *mockStore) GetTestCase(_ context.Context, _ string) (store.TestCase, error) {
	return store.TestCase{}, store.ErrNotFound
}
func (m *mockStore) ListTestCases(_ context.Context, _ string) ([]store.TestCase, error) {
	return nil, nil
}
func (m *mockStore) UpdateTestCase(_ context.Context, tc store.TestCase) (store.TestCase, error) {
	return tc, nil
}
func (m *mockStore) DeleteTestCase(_ context.Context, _ string) error { return nil }
func (m *mockStore) ReorderTestCases(_ context.Context, _ string, _ []string) error { return nil }

func (m *mockStore) CreateEvalRun(_ context.Context, r store.EvalRun) (store.EvalRun, error) {
	return r, nil
}
func (m *mockStore) GetEvalRun(_ context.Context, _ string) (store.EvalRun, error) {
	return store.EvalRun{}, store.ErrNotFound
}
func (m *mockStore) ListEvalRuns(_ context.Context, _ store.EvalRunFilter) ([]store.EvalRun, error) {
	return nil, nil
}
func (m *mockStore) UpdateEvalRunStatus(_ context.Context, _ string, _ store.EvalRunStatus, _ store.EvalRunCounts) error {
	return nil
}
func (m *mockStore) IncrementEvalRunCounts(_ context.Context, _ string, _ store.EvalRunCounts) error {
	return nil
}

func (m *mockStore) CreateTestCaseResult(_ context.Context, r store.TestCaseResult) (store.TestCaseResult, error) {
	return r, nil
}
func (m *mockStore) GetTestCaseResult(_ context.Context, _ string) (store.TestCaseResult, error) {
	return store.TestCaseResult{}, store.ErrNotFound
}
func (m *mockStore) ListTestCaseResults(_ context.Context, _ string) ([]store.TestCaseResult, error) {
	return nil, nil
}
func (m *mockStore) CreateWorkflowVersion(_ context.Context, _ store.Workflow) error { return nil }
func (m *mockStore) GetLatestWorkflowVersionNumber(_ context.Context, _ string) (*int, error) {
	return nil, nil
}
func (m *mockStore) ListWorkflowVersions(_ context.Context, _ string) ([]store.WorkflowVersionSummary, error) {
	return nil, nil
}
func (m *mockStore) GetWorkflowVersion(_ context.Context, _ string, _ int) (store.WorkflowVersion, error) {
	return store.WorkflowVersion{}, store.ErrNotFound
}
func (m *mockStore) DeleteWorkflowVersions(_ context.Context, _ string) error { return nil }
func (m *mockStore) RestoreWorkflowVersion(_ context.Context, _ string, _ int) (store.Workflow, error) {
	return store.Workflow{}, store.ErrNotFound
}

// ---- Auth methods (not used by API handler tests; stub to satisfy store.Store) ----

func (m *mockStore) CreateOrganization(_ context.Context, org store.Organization) (store.Organization, error) {
	return org, nil
}
func (m *mockStore) GetOrganization(_ context.Context, _ string) (store.Organization, error) {
	return store.Organization{}, store.ErrNotFound
}
func (m *mockStore) ListOrganizations(_ context.Context) ([]store.Organization, error) {
	return nil, nil
}
func (m *mockStore) DeleteOrganization(_ context.Context, _ string) error { return nil }

func (m *mockStore) CreateUser(_ context.Context, u store.User) (store.User, error) {
	return u, nil
}
func (m *mockStore) GetUser(_ context.Context, _ string) (store.User, error) {
	return store.User{}, store.ErrNotFound
}
func (m *mockStore) GetUserByEmail(_ context.Context, _ string) (store.User, error) {
	return store.User{}, store.ErrNotFound
}
func (m *mockStore) ListUsers(_ context.Context, _ string) ([]store.User, error) { return nil, nil }
func (m *mockStore) UpdateUserRole(_ context.Context, _, _ string) error          { return nil }
func (m *mockStore) UpdateUserPermissions(_ context.Context, _ string, _ []string) error {
	return nil
}
func (m *mockStore) DeleteUser(_ context.Context, _ string) error { return nil }

func (m *mockStore) CreateInvitation(_ context.Context, inv store.Invitation) (store.Invitation, error) {
	return inv, nil
}
func (m *mockStore) GetInvitationByToken(_ context.Context, _ string) (store.Invitation, error) {
	return store.Invitation{}, store.ErrNotFound
}
func (m *mockStore) AcceptInvitation(_ context.Context, _ string, _ time.Time) error { return nil }
