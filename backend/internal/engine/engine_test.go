package engine

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/g8rswimmer/cogniflow/internal/node"
	"github.com/g8rswimmer/cogniflow/internal/store"
	"github.com/g8rswimmer/cogniflow/internal/trigger"
)

// ---- mock store ----------------------------------------------------------

type mockEngineStore struct {
	workflows map[string]store.Workflow
	runs      map[string]store.Run
	lastStatus store.RunStatus
	lastOutput map[string]any
}

func newMockEngineStore(wf store.Workflow) *mockEngineStore {
	return &mockEngineStore{
		workflows: map[string]store.Workflow{wf.ID: wf},
		runs:      map[string]store.Run{},
	}
}

func (m *mockEngineStore) GetWorkflow(_ context.Context, id string) (store.Workflow, error) {
	wf, ok := m.workflows[id]
	if !ok {
		return store.Workflow{}, store.ErrNotFound
	}
	return wf, nil
}
func (m *mockEngineStore) GetWorkflowSchema(_ context.Context, id string) (json.RawMessage, error) {
	wf, ok := m.workflows[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	return wf.InitialDataSchema, nil
}
func (m *mockEngineStore) CreateRun(_ context.Context, r store.Run) (store.Run, error) {
	if r.ID == "" {
		r.ID = "test-run-id"
	}
	m.runs[r.ID] = r
	return r, nil
}
func (m *mockEngineStore) SaveRunNodeResults(_ context.Context, _ string, _ map[string]store.NodeResult) error {
	return nil
}
func (m *mockEngineStore) UpdateRunStatus(_ context.Context, runID string, status store.RunStatus, output map[string]any) error {
	m.lastStatus = status
	m.lastOutput = output
	r := m.runs[runID]
	r.Status = status
	m.runs[runID] = r
	return nil
}
func (m *mockEngineStore) GetRun(_ context.Context, id string) (store.Run, error) {
	r, ok := m.runs[id]
	if !ok {
		return store.Run{}, store.ErrNotFound
	}
	return r, nil
}
func (m *mockEngineStore) ListRuns(_ context.Context, _ store.RunFilter) ([]store.Run, error) {
	return nil, nil
}

// Unused store methods.
func (m *mockEngineStore) CreateWorkflow(_ context.Context, w store.Workflow) (store.Workflow, error) {
	return w, nil
}
func (m *mockEngineStore) ListWorkflows(_ context.Context) ([]store.WorkflowSummary, error) {
	return nil, nil
}
func (m *mockEngineStore) UpdateWorkflow(_ context.Context, w store.Workflow) (store.Workflow, error) {
	return w, nil
}
func (m *mockEngineStore) DeleteWorkflow(_ context.Context, _ string) error { return nil }
func (m *mockEngineStore) UpsertChunks(_ context.Context, _ []store.RAGChunk) error { return nil }
func (m *mockEngineStore) SearchChunks(_ context.Context, _ []float32, _ int, _ string) ([]store.RAGChunkResult, error) {
	return nil, nil
}
func (m *mockEngineStore) SaveTriggerConfig(_ context.Context, _ string, _ store.TriggerConfig) error {
	return nil
}
func (m *mockEngineStore) GetTriggerConfig(_ context.Context, _ string) (store.TriggerConfig, error) {
	return store.TriggerConfig{}, nil
}
func (m *mockEngineStore) ListTriggerConfigs(_ context.Context) ([]store.WorkflowTrigger, error) {
	return nil, nil
}
func (m *mockEngineStore) SavePluginRegistration(_ context.Context, _ store.PluginRegistration) error {
	return nil
}
func (m *mockEngineStore) GetPluginRegistration(_ context.Context, _ string) (store.PluginRegistration, error) {
	return store.PluginRegistration{}, nil
}
func (m *mockEngineStore) ListPluginRegistrations(_ context.Context) ([]store.PluginRegistration, error) {
	return nil, nil
}
func (m *mockEngineStore) DeletePluginRegistration(_ context.Context, _ string) error { return nil }
func (m *mockEngineStore) CreateEvalSuite(_ context.Context, v store.EvalSuite) (store.EvalSuite, error) {
	return v, nil
}
func (m *mockEngineStore) GetEvalSuite(_ context.Context, _ string) (store.EvalSuite, error) {
	return store.EvalSuite{}, store.ErrNotFound
}
func (m *mockEngineStore) ListEvalSuites(_ context.Context, _ string) ([]store.EvalSuiteSummary, error) {
	return nil, nil
}
func (m *mockEngineStore) ListEvalSuitesByCronTrigger(_ context.Context) ([]store.EvalSuite, error) {
	return nil, nil
}
func (m *mockEngineStore) UpdateEvalSuite(_ context.Context, v store.EvalSuite) (store.EvalSuite, error) {
	return v, nil
}
func (m *mockEngineStore) DeleteEvalSuite(_ context.Context, _ string) error { return nil }
func (m *mockEngineStore) CreateTestCase(_ context.Context, v store.TestCase) (store.TestCase, error) {
	return v, nil
}
func (m *mockEngineStore) GetTestCase(_ context.Context, _ string) (store.TestCase, error) {
	return store.TestCase{}, store.ErrNotFound
}
func (m *mockEngineStore) ListTestCases(_ context.Context, _ string) ([]store.TestCase, error) {
	return nil, nil
}
func (m *mockEngineStore) UpdateTestCase(_ context.Context, v store.TestCase) (store.TestCase, error) {
	return v, nil
}
func (m *mockEngineStore) DeleteTestCase(_ context.Context, _ string) error { return nil }
func (m *mockEngineStore) ReorderTestCases(_ context.Context, _ string, _ []string) error {
	return nil
}
func (m *mockEngineStore) CreateEvalRun(_ context.Context, v store.EvalRun) (store.EvalRun, error) {
	return v, nil
}
func (m *mockEngineStore) GetEvalRun(_ context.Context, _ string) (store.EvalRun, error) {
	return store.EvalRun{}, store.ErrNotFound
}
func (m *mockEngineStore) ListEvalRuns(_ context.Context, _ store.EvalRunFilter) ([]store.EvalRun, error) {
	return nil, nil
}
func (m *mockEngineStore) UpdateEvalRunStatus(_ context.Context, _ string, _ store.EvalRunStatus, _ store.EvalRunCounts) error {
	return nil
}
func (m *mockEngineStore) IncrementEvalRunCounts(_ context.Context, _ string, _ store.EvalRunCounts) error {
	return nil
}
func (m *mockEngineStore) CreateTestCaseResult(_ context.Context, v store.TestCaseResult) (store.TestCaseResult, error) {
	return v, nil
}
func (m *mockEngineStore) GetTestCaseResult(_ context.Context, _ string) (store.TestCaseResult, error) {
	return store.TestCaseResult{}, store.ErrNotFound
}
func (m *mockEngineStore) ListTestCaseResults(_ context.Context, _ string) ([]store.TestCaseResult, error) {
	return nil, nil
}

// ---- helpers -------------------------------------------------------------

func buildTestWorkflow(id string, handlers ...node.NodeHandler) (store.Workflow, *node.NodeRegistry) {
	registry := node.NewRegistry()
	nodes := make([]store.WorkflowNode, len(handlers))
	for i, h := range handlers {
		registry.Register(h)
		nodes[i] = store.WorkflowNode{ID: h.Meta().TypeID, TypeID: h.Meta().TypeID}
	}
	var edges []store.WorkflowEdge
	for i := 1; i < len(nodes); i++ {
		edges = append(edges, store.WorkflowEdge{
			ID:       nodes[i-1].ID + "->" + nodes[i].ID,
			SourceID: nodes[i-1].ID,
			TargetID: nodes[i].ID,
		})
	}
	wf := store.Workflow{
		ID:             id,
		Name:           "test",
		Trigger:        store.Trigger{Kind: "manual"},
		TimeoutSeconds: 30,
		Nodes:          nodes,
		Edges:          edges,
	}
	return wf, registry
}

// ---- tests ---------------------------------------------------------------

func TestWorkflowEngine_Dispatch_ReturnsRunID(t *testing.T) {
	wf, registry := buildTestWorkflow("wf-1",
		&fixedHandler{meta: newMeta("step1"), output: map[string]any{"ok": true}},
	)
	ms := newMockEngineStore(wf)

	bus := NewEventBus()
	eng := NewWorkflowEngine(ms, registry, bus)

	runID, err := eng.Dispatch(context.Background(), trigger.RunRequest{
		WorkflowID:  "wf-1",
		TriggeredBy: "manual",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runID == "" {
		t.Fatal("expected non-empty run ID")
	}
}

func TestWorkflowEngine_Dispatch_WorkflowNotFound(t *testing.T) {
	ms := newMockEngineStore(store.Workflow{ID: "wf-1"})
	bus := NewEventBus()
	eng := NewWorkflowEngine(ms, node.NewRegistry(), bus)

	_, err := eng.Dispatch(context.Background(), trigger.RunRequest{WorkflowID: "does-not-exist"})
	if err == nil {
		t.Fatal("expected error for missing workflow")
	}
}

func TestWorkflowEngine_Run_EventsClosedOnCompletion(t *testing.T) {
	wf, registry := buildTestWorkflow("wf-2",
		&fixedHandler{meta: newMeta("fast"), output: map[string]any{"done": true}},
	)
	ms := newMockEngineStore(wf)
	bus := NewEventBus()
	eng := NewWorkflowEngine(ms, registry, bus)

	handle, err := eng.Run(context.Background(), trigger.RunRequest{
		WorkflowID:  "wf-2",
		TriggeredBy: "manual",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Drain events until the channel closes.
	timeout := time.After(5 * time.Second)
	for {
		select {
		case _, ok := <-handle.Events:
			if !ok {
				return // channel closed — success
			}
		case <-timeout:
			t.Fatal("timeout waiting for run to complete")
		}
	}
}

func TestWorkflowEngine_Run_StatusUpdatedOnSuccess(t *testing.T) {
	wf, registry := buildTestWorkflow("wf-3",
		&fixedHandler{meta: newMeta("ok"), output: map[string]any{"v": 1}},
	)
	ms := newMockEngineStore(wf)
	bus := NewEventBus()
	eng := NewWorkflowEngine(ms, registry, bus)

	handle, err := eng.Run(context.Background(), trigger.RunRequest{
		WorkflowID:  "wf-3",
		TriggeredBy: "manual",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Wait for completion.
	for range handle.Events {
	}

	if ms.lastStatus != store.RunStatusSucceeded {
		t.Errorf("expected succeeded, got %v", ms.lastStatus)
	}
}

func TestWorkflowEngine_Run_StatusUpdatedOnFailure(t *testing.T) {
	wf, registry := buildTestWorkflow("wf-4",
		&failHandler{meta: newMeta("oops")},
	)
	ms := newMockEngineStore(wf)
	bus := NewEventBus()
	eng := NewWorkflowEngine(ms, registry, bus)

	handle, err := eng.Run(context.Background(), trigger.RunRequest{
		WorkflowID:  "wf-4",
		TriggeredBy: "manual",
	})
	if err != nil {
		t.Fatalf("unexpected error from Run: %v", err)
	}

	// Wait for completion.
	for range handle.Events {
	}

	if ms.lastStatus != store.RunStatusFailed {
		t.Errorf("expected failed, got %v", ms.lastStatus)
	}
}
