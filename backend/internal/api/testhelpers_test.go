package api

import (
	"context"
	"errors"
	"sync"

	"github.com/g8rswimmer/cogniflow/internal/store"
	"github.com/g8rswimmer/cogniflow/internal/trigger"
)

// mockStore is an in-memory store.Store implementation for handler tests.
type mockStore struct {
	mu        sync.RWMutex
	workflows map[string]store.Workflow
	runs      map[string]store.Run

	// Per-method error overrides.
	createErr    error
	listErr      error
	getErr       error
	updateErr    error
	deleteErr    error
	createRunErr error
	getRunErr    error
}

func newMockStore() *mockStore {
	return &mockStore{
		workflows: make(map[string]store.Workflow),
		runs:      make(map[string]store.Run),
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
