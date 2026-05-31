package api

import (
	"context"
	"errors"
	"sync"

	"github.com/g8rswimmer/cogniflow/internal/store"
)

// mockStore is an in-memory store.Store implementation for handler tests.
type mockStore struct {
	mu        sync.RWMutex
	workflows map[string]store.Workflow

	// Per-method error overrides.
	createErr error
	listErr   error
	getErr    error
	updateErr error
	deleteErr error
}

func newMockStore() *mockStore {
	return &mockStore{workflows: make(map[string]store.Workflow)}
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

// Stub implementations for unrelated Store methods.

func (m *mockStore) CreateRun(_ context.Context, r store.Run) (store.Run, error) { return r, nil }
func (m *mockStore) UpdateRunStatus(_ context.Context, _ string, _ store.RunStatus, _ map[string]any) error {
	return nil
}
func (m *mockStore) GetRun(_ context.Context, _ string) (store.Run, error) { return store.Run{}, nil }
func (m *mockStore) ListRuns(_ context.Context, _ store.RunFilter) ([]store.Run, error) {
	return nil, nil
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
