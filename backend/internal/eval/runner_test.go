package eval

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/g8rswimmer/cogniflow/internal/crypto"
	"github.com/g8rswimmer/cogniflow/internal/engine"
	"github.com/g8rswimmer/cogniflow/internal/store"
	"github.com/g8rswimmer/cogniflow/internal/trigger"
)

// ---- stub engine -----------------------------------------------------------

type stubEngineRunner struct {
	mu     sync.Mutex
	calls  []trigger.RunRequest
	runs   map[string]stubbedRun
	runErr error
}

type stubbedRun struct {
	runID  string
	status store.RunStatus
	events []engine.NodeEvent
}

func (s *stubEngineRunner) Run(_ context.Context, req trigger.RunRequest) (engine.RunHandle, error) {
	if s.runErr != nil {
		return engine.RunHandle{}, s.runErr
	}
	s.mu.Lock()
	s.calls = append(s.calls, req)
	s.mu.Unlock()

	// Pick the stubbed run based on workflow ID (first match) or use a default.
	ch := make(chan engine.NodeEvent, 32)
	runID := "wf-run-default"
	for _, r := range s.runs {
		if r.runID != "" {
			runID = r.runID
			go func(r stubbedRun) {
				for _, e := range r.events {
					ch <- e
				}
				close(ch)
			}(r)
			goto found
		}
	}
	close(ch)
found:
	return engine.RunHandle{RunID: runID, Events: ch, Cancel: func() {}}, nil
}

// ---- stub store (extends the handler_test stubStore) -----------------------

func newRunnerStore() *runnerStore {
	return &runnerStore{
		suites:    make(map[string]store.EvalSuite),
		testCases: make(map[string][]store.TestCase),
		evalRuns:  make(map[string]store.EvalRun),
		tcResults: make(map[string]store.TestCaseResult),
		wfRuns:    make(map[string]store.Run),
	}
}

type runnerStore struct {
	mu        sync.Mutex
	suites    map[string]store.EvalSuite
	testCases map[string][]store.TestCase // suite_id → ordered list
	evalRuns  map[string]store.EvalRun
	tcResults map[string]store.TestCaseResult
	wfRuns    map[string]store.Run
}

// store.Store interface — implement only what EvalRunner uses.

func (s *runnerStore) GetEvalSuite(_ context.Context, id string) (store.EvalSuite, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if suite, ok := s.suites[id]; ok {
		return suite, nil
	}
	return store.EvalSuite{}, store.ErrNotFound
}
func (s *runnerStore) ListTestCases(_ context.Context, suiteID string) ([]store.TestCase, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.testCases[suiteID], nil
}
func (s *runnerStore) CreateEvalRun(_ context.Context, r store.EvalRun) (store.EvalRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.ID == "" {
		r.ID = "eval-run-id"
	}
	s.evalRuns[r.ID] = r
	return r, nil
}
func (s *runnerStore) UpdateEvalRunStatus(_ context.Context, id string, status store.EvalRunStatus, counts store.EvalRunCounts) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.evalRuns[id]
	r.Status = status
	r.PassedCount = counts.PassedCount
	r.FailedCount = counts.FailedCount
	r.ErrorCount = counts.ErrorCount
	s.evalRuns[id] = r
	return nil
}
func (s *runnerStore) CreateTestCaseResult(_ context.Context, r store.TestCaseResult) (store.TestCaseResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.ID == "" {
		r.ID = "tcr-" + r.TestCaseID
	}
	s.tcResults[r.ID] = r
	return r, nil
}
func (s *runnerStore) GetRun(_ context.Context, runID string) (store.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.wfRuns[runID]; ok {
		return r, nil
	}
	return store.Run{}, store.ErrNotFound
}

// Unused methods — satisfy the store.Store interface.
func (s *runnerStore) CreateWorkflow(_ context.Context, w store.Workflow) (store.Workflow, error) {
	return w, nil
}
func (s *runnerStore) GetWorkflow(_ context.Context, _ string) (store.Workflow, error) {
	return store.Workflow{}, store.ErrNotFound
}
func (s *runnerStore) GetWorkflowSchema(_ context.Context, _ string) (json.RawMessage, error) {
	return nil, nil
}
func (s *runnerStore) ListWorkflows(_ context.Context) ([]store.WorkflowSummary, error) {
	return nil, nil
}
func (s *runnerStore) UpdateWorkflow(_ context.Context, w store.Workflow) (store.Workflow, error) {
	return w, nil
}
func (s *runnerStore) DeleteWorkflow(_ context.Context, _ string) error               { return nil }
func (s *runnerStore) CreateRun(_ context.Context, r store.Run) (store.Run, error)    { return r, nil }
func (s *runnerStore) UpdateRunStatus(_ context.Context, _ string, _ store.RunStatus, _ map[string]any) error {
	return nil
}
func (s *runnerStore) ListRuns(_ context.Context, _ store.RunFilter) ([]store.Run, error) {
	return nil, nil
}
func (s *runnerStore) UpsertChunks(_ context.Context, _ []store.RAGChunk) error { return nil }
func (s *runnerStore) SearchChunks(_ context.Context, _ []float32, _ int, _ string) ([]store.RAGChunkResult, error) {
	return nil, nil
}
func (s *runnerStore) SaveTriggerConfig(_ context.Context, _ string, _ store.TriggerConfig) error {
	return nil
}
func (s *runnerStore) GetTriggerConfig(_ context.Context, _ string) (store.TriggerConfig, error) {
	return store.TriggerConfig{}, nil
}
func (s *runnerStore) ListTriggerConfigs(_ context.Context) ([]store.WorkflowTrigger, error) {
	return nil, nil
}
func (s *runnerStore) SavePluginRegistration(_ context.Context, _ store.PluginRegistration) error {
	return nil
}
func (s *runnerStore) GetPluginRegistration(_ context.Context, _ string) (store.PluginRegistration, error) {
	return store.PluginRegistration{}, store.ErrNotFound
}
func (s *runnerStore) ListPluginRegistrations(_ context.Context) ([]store.PluginRegistration, error) {
	return nil, nil
}
func (s *runnerStore) DeletePluginRegistration(_ context.Context, _ string) error { return nil }
func (s *runnerStore) CreateEvalSuite(_ context.Context, suite store.EvalSuite) (store.EvalSuite, error) {
	return suite, nil
}
func (s *runnerStore) ListEvalSuites(_ context.Context, _ string) ([]store.EvalSuiteSummary, error) {
	return nil, nil
}
func (s *runnerStore) UpdateEvalSuite(_ context.Context, suite store.EvalSuite) (store.EvalSuite, error) {
	return suite, nil
}
func (s *runnerStore) DeleteEvalSuite(_ context.Context, _ string) error { return nil }
func (s *runnerStore) CreateTestCase(_ context.Context, tc store.TestCase) (store.TestCase, error) {
	return tc, nil
}
func (s *runnerStore) GetTestCase(_ context.Context, _ string) (store.TestCase, error) {
	return store.TestCase{}, store.ErrNotFound
}
func (s *runnerStore) UpdateTestCase(_ context.Context, tc store.TestCase) (store.TestCase, error) {
	return tc, nil
}
func (s *runnerStore) DeleteTestCase(_ context.Context, _ string) error { return nil }
func (s *runnerStore) ReorderTestCases(_ context.Context, _ string, _ []string) error { return nil }
func (s *runnerStore) GetEvalRun(_ context.Context, id string) (store.EvalRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.evalRuns[id]; ok {
		return r, nil
	}
	return store.EvalRun{}, store.ErrNotFound
}
func (s *runnerStore) ListEvalRuns(_ context.Context, _ store.EvalRunFilter) ([]store.EvalRun, error) {
	return nil, nil
}
func (s *runnerStore) IncrementEvalRunCounts(_ context.Context, _ string, _ store.EvalRunCounts) error {
	return nil
}
func (s *runnerStore) GetTestCaseResult(_ context.Context, id string) (store.TestCaseResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.tcResults[id]; ok {
		return r, nil
	}
	return store.TestCaseResult{}, store.ErrNotFound
}
func (s *runnerStore) ListTestCaseResults(_ context.Context, _ string) ([]store.TestCaseResult, error) {
	return nil, nil
}

// ---- helpers ---------------------------------------------------------------

func newTestRunner(t *testing.T) (*EvalRunner, *runnerStore, *stubEngineRunner) {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	c, err := crypto.NewCipher(key)
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	vault := NewGraderVault(c)
	st := newRunnerStore()
	eng := &stubEngineRunner{runs: make(map[string]stubbedRun)}
	r := &EvalRunner{store: st, engine: eng, vault: vault, ctx: context.Background()}
	return r, st, eng
}

// waitFor polls f until it returns true or the deadline is reached.
func waitFor(t *testing.T, f func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if f() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out waiting for condition")
}

// ---- tests -----------------------------------------------------------------

func TestEvalRunner_Execute_ReturnsSuiteNotFound(t *testing.T) {
	r, _, _ := newTestRunner(t)
	_, err := r.Execute(context.Background(), "missing-suite")
	if err == nil {
		t.Error("expected error for missing suite")
	}
}

func TestEvalRunner_Execute_ReturnsRunID(t *testing.T) {
	r, st, eng := newTestRunner(t)

	st.mu.Lock()
	st.suites["es-1"] = store.EvalSuite{ID: "es-1", WorkflowID: "wf-1", Name: "Suite", PassThreshold: 1.0, MaxConcurrency: 1}
	st.testCases["es-1"] = []store.TestCase{} // no test cases
	st.mu.Unlock()

	eng.runs["default"] = stubbedRun{runID: "wf-run-1", status: store.RunStatusSucceeded}

	runID, err := r.Execute(context.Background(), "es-1")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if runID == "" {
		t.Error("expected non-empty run ID")
	}
}

func TestEvalRunner_Execute_PassingStringMatchGrader(t *testing.T) {
	r, st, eng := newTestRunner(t)

	suite := store.EvalSuite{
		ID: "es-1", WorkflowID: "wf-1", Name: "Suite", PassThreshold: 1.0, MaxConcurrency: 1,
	}
	tc := store.TestCase{
		ID:          "tc-1",
		SuiteID:     "es-1",
		Name:        "Happy path",
		InitialData: map[string]any{},
		Graders: []store.GraderDef{{
			ID: "g1", Name: "Check completion", Type: "string_match", Scope: "workflow",
			Config: map[string]any{
				"field_path":     "n1.completion",
				"match_type":     "contains",
				"expected_value": "hello",
			},
		}},
	}

	st.mu.Lock()
	st.suites["es-1"] = suite
	st.testCases["es-1"] = []store.TestCase{tc}
	st.wfRuns["wf-run-1"] = store.Run{
		ID:          "wf-run-1",
		Status:      store.RunStatusSucceeded,
		FinalOutput: map[string]any{"n1": map[string]any{"completion": "hello world"}},
	}
	st.mu.Unlock()

	eng.runs["r1"] = stubbedRun{
		runID:  "wf-run-1",
		
		events: []engine.NodeEvent{
			{RunID: "wf-run-1", NodeID: "n1", Type: engine.EventRunSucceeded},
		},
	}

	runID, err := r.Execute(context.Background(), "es-1")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// Wait for the async run to complete.
	waitFor(t, func() bool {
		st.mu.Lock()
		defer st.mu.Unlock()
		run, ok := st.evalRuns[runID]
		return ok && run.Status == store.EvalRunCompleted
	})

	st.mu.Lock()
	run := st.evalRuns[runID]
	st.mu.Unlock()

	if run.PassedCount != 1 {
		t.Errorf("want passed_count=1, got %d", run.PassedCount)
	}
	if run.FailedCount != 0 {
		t.Errorf("want failed_count=0, got %d", run.FailedCount)
	}
}

func TestEvalRunner_Execute_FailingGrader(t *testing.T) {
	r, st, eng := newTestRunner(t)

	suite := store.EvalSuite{
		ID: "es-1", WorkflowID: "wf-1", Name: "Suite", PassThreshold: 1.0, MaxConcurrency: 1,
	}
	tc := store.TestCase{
		ID:          "tc-1",
		SuiteID:     "es-1",
		Name:        "Should fail",
		InitialData: map[string]any{},
		Graders: []store.GraderDef{{
			ID: "g1", Name: "Must contain XYZZY", Type: "string_match", Scope: "workflow",
			Config: map[string]any{
				"field_path":     "n1.completion",
				"match_type":     "contains",
				"expected_value": "XYZZY",
			},
		}},
	}

	st.mu.Lock()
	st.suites["es-1"] = suite
	st.testCases["es-1"] = []store.TestCase{tc}
	st.wfRuns["wf-run-1"] = store.Run{
		ID:          "wf-run-1",
		Status:      store.RunStatusSucceeded,
		FinalOutput: map[string]any{"n1": map[string]any{"completion": "no magic word here"}},
	}
	st.mu.Unlock()

	eng.runs["r1"] = stubbedRun{runID: "wf-run-1", status: store.RunStatusSucceeded}

	runID, err := r.Execute(context.Background(), "es-1")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	waitFor(t, func() bool {
		st.mu.Lock()
		defer st.mu.Unlock()
		run, ok := st.evalRuns[runID]
		return ok && run.Status == store.EvalRunCompleted
	})

	st.mu.Lock()
	run := st.evalRuns[runID]
	st.mu.Unlock()

	if run.FailedCount != 1 {
		t.Errorf("want failed_count=1, got %d", run.FailedCount)
	}
}

func TestEvalRunner_Execute_WorkflowRunFailed_CountsAsError(t *testing.T) {
	r, st, eng := newTestRunner(t)

	suite := store.EvalSuite{
		ID: "es-1", WorkflowID: "wf-1", Name: "Suite", PassThreshold: 1.0, MaxConcurrency: 1,
	}
	tc := store.TestCase{
		ID:      "tc-1",
		SuiteID: "es-1",
		Name:    "Error case",
		Graders: []store.GraderDef{{
			ID: "g1", Type: "string_match", Scope: "workflow",
			Config: map[string]any{"field_path": "x", "match_type": "exact", "expected_value": "y"},
		}},
	}

	st.mu.Lock()
	st.suites["es-1"] = suite
	st.testCases["es-1"] = []store.TestCase{tc}
	st.wfRuns["wf-run-1"] = store.Run{ID: "wf-run-1", Status: store.RunStatusFailed}
	st.mu.Unlock()

	eng.runs["r1"] = stubbedRun{runID: "wf-run-1", status: store.RunStatusFailed}

	runID, _ := r.Execute(context.Background(), "es-1")

	waitFor(t, func() bool {
		st.mu.Lock()
		defer st.mu.Unlock()
		run, ok := st.evalRuns[runID]
		return ok && run.Status == store.EvalRunCompleted
	})

	st.mu.Lock()
	run := st.evalRuns[runID]
	st.mu.Unlock()

	if run.ErrorCount != 1 {
		t.Errorf("want error_count=1, got %d", run.ErrorCount)
	}
}

func TestEvalRunner_Execute_NodeMock_CapturesOutput(t *testing.T) {
	r, st, eng := newTestRunner(t)

	suite := store.EvalSuite{
		ID: "es-1", WorkflowID: "wf-1", Name: "Suite", PassThreshold: 1.0, MaxConcurrency: 1,
	}
	tc := store.TestCase{
		ID:      "tc-1",
		SuiteID: "es-1",
		Name:    "Mock test",
		Mocks: []store.NodeMock{
			{NodeID: "http-1", Output: map[string]any{"status_code": float64(200), "body": "ok"}},
		},
		Graders: []store.GraderDef{{
			ID: "g1", Name: "Status 200", Type: "numeric_threshold", Scope: "node", NodeID: "http-1",
			Config: map[string]any{"field_path": "status_code", "operator": "==", "threshold": float64(200)},
		}},
	}

	st.mu.Lock()
	st.suites["es-1"] = suite
	st.testCases["es-1"] = []store.TestCase{tc}
	st.wfRuns["wf-run-1"] = store.Run{ID: "wf-run-1", Status: store.RunStatusSucceeded}
	st.mu.Unlock()

	// Stub engine emits a node.succeeded event carrying the mock output.
	eng.runs["r1"] = stubbedRun{
		runID:  "wf-run-1",
		
		events: []engine.NodeEvent{
			{
				RunID:  "wf-run-1",
				NodeID: "http-1",
				Type:   engine.EventNodeSucceeded,
				Output: map[string]any{"status_code": float64(200), "body": "ok", "mocked": true},
			},
			{RunID: "wf-run-1", Type: engine.EventRunSucceeded},
		},
	}

	runID, err := r.Execute(context.Background(), "es-1")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	waitFor(t, func() bool {
		st.mu.Lock()
		defer st.mu.Unlock()
		run, ok := st.evalRuns[runID]
		return ok && run.Status == store.EvalRunCompleted
	})

	// Verify the grader passed (numeric 200 == 200).
	var tcResult store.TestCaseResult
	st.mu.Lock()
	for _, r := range st.tcResults {
		if r.EvalRunID == runID {
			tcResult = r
			break
		}
	}
	st.mu.Unlock()

	if len(tcResult.GraderResults) == 0 {
		t.Fatal("expected grader results")
	}
	if tcResult.GraderResults[0].Verdict != store.VerdictPass {
		t.Errorf("want pass, got %s: %s", tcResult.GraderResults[0].Verdict, tcResult.GraderResults[0].Explanation)
	}
	// Verify node_outputs captured the mock output.
	nodeOut := tcResult.NodeOutputs["http-1"]
	if nodeOut == nil {
		t.Error("expected http-1 node output to be captured")
	}
}

func TestEvalRunner_Execute_ZeroGraders_SmokeTest(t *testing.T) {
	r, st, eng := newTestRunner(t)

	suite := store.EvalSuite{
		ID: "es-1", WorkflowID: "wf-1", Name: "Suite", PassThreshold: 1.0, MaxConcurrency: 1,
	}
	tc := store.TestCase{
		ID:      "tc-1",
		SuiteID: "es-1",
		Name:    "Smoke test",
		Graders: []store.GraderDef{},
	}

	st.mu.Lock()
	st.suites["es-1"] = suite
	st.testCases["es-1"] = []store.TestCase{tc}
	st.wfRuns["wf-run-1"] = store.Run{ID: "wf-run-1", Status: store.RunStatusSucceeded}
	st.mu.Unlock()

	eng.runs["r1"] = stubbedRun{runID: "wf-run-1", status: store.RunStatusSucceeded}

	runID, _ := r.Execute(context.Background(), "es-1")

	waitFor(t, func() bool {
		st.mu.Lock()
		defer st.mu.Unlock()
		run, ok := st.evalRuns[runID]
		return ok && run.Status == store.EvalRunCompleted
	})

	st.mu.Lock()
	run := st.evalRuns[runID]
	st.mu.Unlock()

	if run.PassedCount != 1 {
		t.Errorf("zero-grader smoke test should pass; got passed=%d, failed=%d", run.PassedCount, run.FailedCount)
	}
}
