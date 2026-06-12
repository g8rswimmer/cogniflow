package mysql

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/g8rswimmer/cogniflow/internal/store"
)

// ---- helpers ---------------------------------------------------------------

func insertTestSuite(t *testing.T, s *WorkflowStore, suite store.EvalSuite) store.EvalSuite {
	t.Helper()
	got, err := s.CreateEvalSuite(context.Background(), suite)
	if err != nil {
		t.Fatalf("CreateEvalSuite: %v", err)
	}
	return got
}

func insertTestCase(t *testing.T, s *WorkflowStore, tc store.TestCase) store.TestCase {
	t.Helper()
	got, err := s.CreateTestCase(context.Background(), tc)
	if err != nil {
		t.Fatalf("CreateTestCase: %v", err)
	}
	return got
}

// ---- EvalSuite tests -------------------------------------------------------

func TestEvalStore_CreateGetSuite(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	in := store.EvalSuite{
		WorkflowID:    "wf-1",
		Name:          "Smoke Suite",
		Description:   "basic checks",
		PassThreshold: 0.8,
		MaxConcurrency: 2,
	}
	got, err := s.CreateEvalSuite(ctx, in)
	if err != nil {
		t.Fatalf("CreateEvalSuite: %v", err)
	}
	if got.ID == "" {
		t.Error("expected ID to be set")
	}
	if got.PassThreshold != 0.8 {
		t.Errorf("pass_threshold: want 0.8, got %f", got.PassThreshold)
	}
	if got.MaxConcurrency != 2 {
		t.Errorf("max_concurrency: want 2, got %d", got.MaxConcurrency)
	}

	fetched, err := s.GetEvalSuite(ctx, got.ID)
	if err != nil {
		t.Fatalf("GetEvalSuite: %v", err)
	}
	if fetched.Name != "Smoke Suite" {
		t.Errorf("name: want Smoke Suite, got %q", fetched.Name)
	}
}

func TestEvalStore_GetSuite_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetEvalSuite(context.Background(), "no-such-id")
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestEvalStore_ListSuites(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	insertTestSuite(t, s, store.EvalSuite{WorkflowID: "wf-1", Name: "Suite A"})
	insertTestSuite(t, s, store.EvalSuite{WorkflowID: "wf-1", Name: "Suite B"})
	insertTestSuite(t, s, store.EvalSuite{WorkflowID: "wf-2", Name: "Other"})

	suites, err := s.ListEvalSuites(ctx, "wf-1")
	if err != nil {
		t.Fatalf("ListEvalSuites: %v", err)
	}
	if len(suites) != 2 {
		t.Errorf("want 2 suites, got %d", len(suites))
	}
	for _, su := range suites {
		if su.WorkflowID != "wf-1" {
			t.Errorf("unexpected workflow_id %q", su.WorkflowID)
		}
	}
}

func TestEvalStore_ListSuites_WithCounts(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	suite := insertTestSuite(t, s, store.EvalSuite{WorkflowID: "wf-1", Name: "S"})
	insertTestCase(t, s, store.TestCase{SuiteID: suite.ID, Name: "TC1"})
	insertTestCase(t, s, store.TestCase{SuiteID: suite.ID, Name: "TC2"})

	summaries, err := s.ListEvalSuites(ctx, "wf-1")
	if err != nil {
		t.Fatalf("ListEvalSuites: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("want 1 summary, got %d", len(summaries))
	}
	if summaries[0].TestCaseCount != 2 {
		t.Errorf("test_case_count: want 2, got %d", summaries[0].TestCaseCount)
	}
	if summaries[0].LastRunStatus != nil {
		t.Errorf("last_run_status: want nil, got %v", *summaries[0].LastRunStatus)
	}
}

func TestEvalStore_UpdateSuite(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	orig := insertTestSuite(t, s, store.EvalSuite{WorkflowID: "wf-1", Name: "Old Name"})

	orig.Name = "New Name"
	orig.PassThreshold = 0.5

	updated, err := s.UpdateEvalSuite(ctx, orig)
	if err != nil {
		t.Fatalf("UpdateEvalSuite: %v", err)
	}
	if updated.Name != "New Name" {
		t.Errorf("name: want New Name, got %q", updated.Name)
	}
	if updated.PassThreshold != 0.5 {
		t.Errorf("pass_threshold: want 0.5, got %f", updated.PassThreshold)
	}
}

func TestEvalStore_UpdateSuite_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.UpdateEvalSuite(context.Background(), store.EvalSuite{ID: "ghost", Name: "x"})
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestEvalStore_DeleteSuite_Cascades(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	suite := insertTestSuite(t, s, store.EvalSuite{WorkflowID: "wf-1", Name: "S"})
	tc := insertTestCase(t, s, store.TestCase{SuiteID: suite.ID, Name: "TC"})

	run, err := s.CreateEvalRun(ctx, store.EvalRun{
		SuiteID:    suite.ID,
		Status:     store.EvalRunPending,
		TotalCases: 1,
	})
	if err != nil {
		t.Fatalf("CreateEvalRun: %v", err)
	}

	_, err = s.CreateTestCaseResult(ctx, store.TestCaseResult{
		EvalRunID:         run.ID,
		TestCaseID:        tc.ID,
		TestCaseName:      tc.Name,
		WorkflowRunID:     "run-1",
		WorkflowRunStatus: "succeeded",
	})
	if err != nil {
		t.Fatalf("CreateTestCaseResult: %v", err)
	}

	if err := s.DeleteEvalSuite(ctx, suite.ID); err != nil {
		t.Fatalf("DeleteEvalSuite: %v", err)
	}

	_, err = s.GetEvalSuite(ctx, suite.ID)
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("suite should be deleted, got %v", err)
	}
	_, err = s.GetEvalRun(ctx, run.ID)
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("eval run should be deleted, got %v", err)
	}
}

func TestEvalStore_DeleteSuite_NotFound(t *testing.T) {
	s := newTestStore(t)
	err := s.DeleteEvalSuite(context.Background(), "ghost")
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

// ---- TestCase tests --------------------------------------------------------

func TestEvalStore_CreateGetTestCase(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	suite := insertTestSuite(t, s, store.EvalSuite{WorkflowID: "wf-1", Name: "S"})

	tc := store.TestCase{
		SuiteID:     suite.ID,
		Name:        "Happy path",
		Description: "all good",
		InitialData: map[string]any{"ticket": "billing"},
		Mocks: []store.NodeMock{
			{NodeID: "n1", Output: map[string]any{"status": 200}},
		},
		Graders: []store.GraderDef{
			{ID: "g1", Name: "check", Type: "string_match", Scope: "workflow",
				Config: map[string]any{"field_path": "completion", "match_type": "contains", "expected_value": "ok"}},
		},
	}

	got, err := s.CreateTestCase(ctx, tc)
	if err != nil {
		t.Fatalf("CreateTestCase: %v", err)
	}
	if got.ID == "" {
		t.Error("expected ID")
	}

	fetched, err := s.GetTestCase(ctx, got.ID)
	if err != nil {
		t.Fatalf("GetTestCase: %v", err)
	}
	if fetched.Name != "Happy path" {
		t.Errorf("name: want Happy path, got %q", fetched.Name)
	}
	if len(fetched.Mocks) != 1 || fetched.Mocks[0].NodeID != "n1" {
		t.Errorf("mocks not round-tripped correctly: %+v", fetched.Mocks)
	}
	if len(fetched.Graders) != 1 || fetched.Graders[0].Type != "string_match" {
		t.Errorf("graders not round-tripped: %+v", fetched.Graders)
	}
	if fetched.InitialData["ticket"] != "billing" {
		t.Errorf("initial_data not round-tripped: %+v", fetched.InitialData)
	}
}

func TestEvalStore_ListTestCases_OrderedByPosition(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	suite := insertTestSuite(t, s, store.EvalSuite{WorkflowID: "wf-1", Name: "S"})
	insertTestCase(t, s, store.TestCase{SuiteID: suite.ID, Name: "First", Position: 0})
	insertTestCase(t, s, store.TestCase{SuiteID: suite.ID, Name: "Second", Position: 1})

	cases, err := s.ListTestCases(ctx, suite.ID)
	if err != nil {
		t.Fatalf("ListTestCases: %v", err)
	}
	if len(cases) != 2 {
		t.Fatalf("want 2, got %d", len(cases))
	}
	if cases[0].Name != "First" {
		t.Errorf("want First first, got %q", cases[0].Name)
	}
}

func TestEvalStore_UpdateTestCase(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	suite := insertTestSuite(t, s, store.EvalSuite{WorkflowID: "wf-1", Name: "S"})
	orig := insertTestCase(t, s, store.TestCase{SuiteID: suite.ID, Name: "Old"})

	orig.Name = "New"
	orig.InitialData = map[string]any{"x": 1}

	updated, err := s.UpdateTestCase(ctx, orig)
	if err != nil {
		t.Fatalf("UpdateTestCase: %v", err)
	}
	if updated.Name != "New" {
		t.Errorf("want New, got %q", updated.Name)
	}
	if updated.InitialData["x"] != float64(1) {
		t.Errorf("initial_data not updated: %+v", updated.InitialData)
	}
}

func TestEvalStore_DeleteTestCase(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	suite := insertTestSuite(t, s, store.EvalSuite{WorkflowID: "wf-1", Name: "S"})
	tc := insertTestCase(t, s, store.TestCase{SuiteID: suite.ID, Name: "TC"})

	if err := s.DeleteTestCase(ctx, tc.ID); err != nil {
		t.Fatalf("DeleteTestCase: %v", err)
	}
	_, err := s.GetTestCase(ctx, tc.ID)
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestEvalStore_ReorderTestCases(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	suite := insertTestSuite(t, s, store.EvalSuite{WorkflowID: "wf-1", Name: "S"})
	tc1 := insertTestCase(t, s, store.TestCase{SuiteID: suite.ID, Name: "A", Position: 0})
	tc2 := insertTestCase(t, s, store.TestCase{SuiteID: suite.ID, Name: "B", Position: 1})

	// Reverse order: B first, A second
	if err := s.ReorderTestCases(ctx, suite.ID, []string{tc2.ID, tc1.ID}); err != nil {
		t.Fatalf("ReorderTestCases: %v", err)
	}

	cases, _ := s.ListTestCases(ctx, suite.ID)
	if cases[0].Name != "B" {
		t.Errorf("want B first, got %q", cases[0].Name)
	}
}

// ---- EvalRun tests ---------------------------------------------------------

func TestEvalStore_CreateGetEvalRun(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	suite := insertTestSuite(t, s, store.EvalSuite{WorkflowID: "wf-1", Name: "S"})

	run, err := s.CreateEvalRun(ctx, store.EvalRun{
		SuiteID:    suite.ID,
		Status:     store.EvalRunPending,
		TotalCases: 3,
	})
	if err != nil {
		t.Fatalf("CreateEvalRun: %v", err)
	}
	if run.ID == "" {
		t.Error("expected ID")
	}

	fetched, err := s.GetEvalRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetEvalRun: %v", err)
	}
	if fetched.Status != store.EvalRunPending {
		t.Errorf("status: want pending, got %q", fetched.Status)
	}
	if fetched.TotalCases != 3 {
		t.Errorf("total_cases: want 3, got %d", fetched.TotalCases)
	}
}

func TestEvalStore_UpdateEvalRunStatus(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	suite := insertTestSuite(t, s, store.EvalSuite{WorkflowID: "wf-1", Name: "S"})
	run, _ := s.CreateEvalRun(ctx, store.EvalRun{SuiteID: suite.ID, Status: store.EvalRunPending, TotalCases: 2})

	err := s.UpdateEvalRunStatus(ctx, run.ID, store.EvalRunCompleted, store.EvalRunCounts{
		PassedCount: 2,
		FailedCount: 0,
		ErrorCount:  0,
	})
	if err != nil {
		t.Fatalf("UpdateEvalRunStatus: %v", err)
	}

	fetched, _ := s.GetEvalRun(ctx, run.ID)
	if fetched.Status != store.EvalRunCompleted {
		t.Errorf("status: want completed, got %q", fetched.Status)
	}
	if fetched.PassedCount != 2 {
		t.Errorf("passed_count: want 2, got %d", fetched.PassedCount)
	}
	if fetched.FinishedAt == nil {
		t.Error("finished_at should be set")
	}
}

func TestEvalStore_IncrementEvalRunCounts(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	suite := insertTestSuite(t, s, store.EvalSuite{WorkflowID: "wf-1", Name: "S"})
	run, _ := s.CreateEvalRun(ctx, store.EvalRun{SuiteID: suite.ID, Status: store.EvalRunRunning, TotalCases: 4})

	if err := s.IncrementEvalRunCounts(ctx, run.ID, store.EvalRunCounts{PassedCount: 1}); err != nil {
		t.Fatalf("IncrementEvalRunCounts: %v", err)
	}
	if err := s.IncrementEvalRunCounts(ctx, run.ID, store.EvalRunCounts{PassedCount: 1}); err != nil {
		t.Fatalf("IncrementEvalRunCounts 2nd: %v", err)
	}
	if err := s.IncrementEvalRunCounts(ctx, run.ID, store.EvalRunCounts{FailedCount: 1}); err != nil {
		t.Fatalf("IncrementEvalRunCounts fail: %v", err)
	}

	fetched, _ := s.GetEvalRun(ctx, run.ID)
	if fetched.PassedCount != 2 {
		t.Errorf("passed_count: want 2, got %d", fetched.PassedCount)
	}
	if fetched.FailedCount != 1 {
		t.Errorf("failed_count: want 1, got %d", fetched.FailedCount)
	}
}

// ---- TestCaseResult tests --------------------------------------------------

func TestEvalStore_CreateGetTestCaseResult(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	suite := insertTestSuite(t, s, store.EvalSuite{WorkflowID: "wf-1", Name: "S"})
	tc := insertTestCase(t, s, store.TestCase{SuiteID: suite.ID, Name: "TC"})
	run, _ := s.CreateEvalRun(ctx, store.EvalRun{SuiteID: suite.ID, Status: store.EvalRunCompleted, TotalCases: 1})

	verdict := store.VerdictPass
	score := 1.0
	result := store.TestCaseResult{
		EvalRunID:         run.ID,
		TestCaseID:        tc.ID,
		TestCaseName:      tc.Name,
		WorkflowRunID:     "wfrun-1",
		WorkflowRunStatus: "succeeded",
		NodeOutputs:       map[string]map[string]any{"n1": {"out": "val"}},
		GraderResults: []store.GraderResult{
			{
				GraderID:    "g1",
				GraderName:  "check",
				GraderType:  "string_match",
				Verdict:     verdict,
				Score:       &score,
				Explanation: "matched",
				ActualValue: "hello",
			},
		},
		Passed: true,
	}

	created, err := s.CreateTestCaseResult(ctx, result)
	if err != nil {
		t.Fatalf("CreateTestCaseResult: %v", err)
	}
	if created.ID == "" {
		t.Error("expected ID")
	}

	fetched, err := s.GetTestCaseResult(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetTestCaseResult: %v", err)
	}
	if !fetched.Passed {
		t.Error("want passed=true")
	}
	if len(fetched.GraderResults) != 1 {
		t.Fatalf("want 1 grader result, got %d", len(fetched.GraderResults))
	}
	if fetched.GraderResults[0].Verdict != store.VerdictPass {
		t.Errorf("verdict: want pass, got %q", fetched.GraderResults[0].Verdict)
	}
	if fetched.NodeOutputs["n1"]["out"] != "val" {
		t.Errorf("node_outputs not round-tripped: %+v", fetched.NodeOutputs)
	}
}

func TestEvalStore_ListTestCaseResults(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	suite := insertTestSuite(t, s, store.EvalSuite{WorkflowID: "wf-1", Name: "S"})
	tc1 := insertTestCase(t, s, store.TestCase{SuiteID: suite.ID, Name: "TC1"})
	tc2 := insertTestCase(t, s, store.TestCase{SuiteID: suite.ID, Name: "TC2"})
	run, _ := s.CreateEvalRun(ctx, store.EvalRun{SuiteID: suite.ID, Status: store.EvalRunCompleted, TotalCases: 2})

	for _, tc := range []store.TestCase{tc1, tc2} {
		_, err := s.CreateTestCaseResult(ctx, store.TestCaseResult{
			EvalRunID: run.ID, TestCaseID: tc.ID, TestCaseName: tc.Name,
			WorkflowRunID: "r", WorkflowRunStatus: "succeeded", Passed: true,
		})
		if err != nil {
			t.Fatalf("CreateTestCaseResult: %v", err)
		}
		time.Sleep(time.Millisecond) // ensure ordered by created_at
	}

	results, err := s.ListTestCaseResults(ctx, run.ID)
	if err != nil {
		t.Fatalf("ListTestCaseResults: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("want 2 results, got %d", len(results))
	}
}
