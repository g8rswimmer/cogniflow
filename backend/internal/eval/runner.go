package eval

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/g8rswimmer/cogniflow/internal/engine"
	"github.com/g8rswimmer/cogniflow/internal/store"
	"github.com/g8rswimmer/cogniflow/internal/trigger"
)

// engineRunner is the minimal interface the eval runner needs from the workflow engine.
// *engine.WorkflowEngine satisfies it.
type engineRunner interface {
	Run(ctx context.Context, req trigger.RunRequest) (engine.RunHandle, error)
}

// EvalRunner orchestrates async execution of an EvalSuite: one workflow run per
// test case, mock interception, grader evaluation, and result persistence.
type EvalRunner struct {
	store      store.Store
	engine     engineRunner
	vault      *GraderVault
	llmFactory LLMFactory
	ctx        context.Context // server-lifetime context; cancelled on shutdown
}

// NewEvalRunner creates an EvalRunner. ctx should be a server-lifetime context
// so that background eval goroutines are cancelled when the server shuts down.
// factory provides LLMClient instances for llm_judge and checklist graders; pass nil
// to disable LLM graders (they will return an error verdict at evaluation time).
func NewEvalRunner(ctx context.Context, st store.Store, eng *engine.WorkflowEngine, vault *GraderVault, factory LLMFactory) *EvalRunner {
	return &EvalRunner{store: st, engine: eng, vault: vault, llmFactory: factory, ctx: ctx}
}

// Execute creates an EvalRun record, starts async execution, and returns the run ID immediately.
func (r *EvalRunner) Execute(ctx context.Context, suiteID string) (string, error) {
	suite, err := r.store.GetEvalSuite(ctx, suiteID)
	if err != nil {
		return "", fmt.Errorf("eval runner: get suite: %w", err)
	}
	if suite.WorkflowDeleted {
		return "", fmt.Errorf("eval runner: workflow has been deleted; cannot trigger run")
	}

	testCases, err := r.store.ListTestCases(ctx, suiteID)
	if err != nil {
		return "", fmt.Errorf("eval runner: list test cases: %w", err)
	}

	// Decrypt grader api_keys before handing to the runner goroutine.
	for i, tc := range testCases {
		dec, err := r.vault.DecryptGraders(tc.Graders)
		if err != nil {
			return "", fmt.Errorf("eval runner: decrypt graders for test case %q: %w", tc.ID, err)
		}
		testCases[i].Graders = dec
	}

	evalRun, err := r.store.CreateEvalRun(ctx, store.EvalRun{
		ID:         uuid.New().String(),
		SuiteID:    suiteID,
		Status:     store.EvalRunPending,
		TotalCases: len(testCases),
	})
	if err != nil {
		return "", fmt.Errorf("eval runner: create eval run: %w", err)
	}

	go r.runAsync(r.ctx, evalRun.ID, suite, testCases)

	return evalRun.ID, nil
}

// runAsync drives the full eval run in a background goroutine.
func (r *EvalRunner) runAsync(ctx context.Context, evalRunID string, suite store.EvalSuite, testCases []store.TestCase) {
	if err := r.store.UpdateEvalRunStatus(ctx, evalRunID, store.EvalRunRunning, store.EvalRunCounts{}); err != nil {
		slog.Error("eval runner: update status to running", "eval_run_id", evalRunID, "error", err)
	}

	concurrency := suite.MaxConcurrency
	if concurrency <= 0 {
		concurrency = 1
	}

	type outcome struct {
		passed bool
		isErr  bool
	}
	outcomes := make([]outcome, len(testCases))

	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrency)

	for i, tc := range testCases {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, tc store.TestCase) {
			defer wg.Done()
			defer func() { <-sem }()

			result, passed, isErr := r.executeTestCase(ctx, evalRunID, tc, suite)
			outcomes[idx] = outcome{passed: passed, isErr: isErr}

			if _, err := r.store.CreateTestCaseResult(ctx, result); err != nil {
				slog.Error("eval runner: persist test case result",
					"eval_run_id", evalRunID,
					"test_case_id", tc.ID,
					"error", err,
				)
			}
		}(i, tc)
	}
	wg.Wait()

	var passed, failed, errCount int
	for _, o := range outcomes {
		switch {
		case o.isErr:
			errCount++
		case o.passed:
			passed++
		default:
			failed++
		}
	}

	finalCounts := store.EvalRunCounts{PassedCount: passed, FailedCount: failed, ErrorCount: errCount}
	if err := r.store.UpdateEvalRunStatus(ctx, evalRunID, store.EvalRunCompleted, finalCounts); err != nil {
		slog.Error("eval runner: update status to completed", "eval_run_id", evalRunID, "error", err)
	}
}

// executeTestCase runs one test case and returns its result, whether it passed, and whether it errored.
func (r *EvalRunner) executeTestCase(ctx context.Context, evalRunID string, tc store.TestCase, suite store.EvalSuite) (store.TestCaseResult, bool, bool) {
	// Build node mocks map.
	nodeMocks := make(map[string]map[string]any, len(tc.Mocks))
	for _, m := range tc.Mocks {
		nodeMocks[m.NodeID] = m.Output
	}

	// Determine which node IDs need output capture (node-scoped graders only).
	captureNodes := make(map[string]bool)
	for _, g := range tc.Graders {
		if g.Scope == "node" && g.NodeID != "" {
			captureNodes[g.NodeID] = true
		}
	}

	req := trigger.RunRequest{
		WorkflowID:  suite.WorkflowID,
		InitialData: tc.InitialData,
		TriggeredBy: "eval",
		NodeMocks:   nodeMocks,
	}

	handle, err := r.engine.Run(ctx, req)
	if err != nil {
		slog.Error("eval runner: engine.Run failed", "eval_run_id", evalRunID, "test_case_id", tc.ID, "error", err)
		return store.TestCaseResult{
			ID:                uuid.New().String(),
			EvalRunID:         evalRunID,
			TestCaseID:        tc.ID,
			TestCaseName:      tc.Name,
			WorkflowRunID:     "",
			WorkflowRunStatus: string(store.RunStatusFailed),
			NodeOutputs:       map[string]map[string]any{},
			GraderResults:     errorGraderResults(tc.Graders, "workflow run could not start: "+err.Error()),
			Passed:            false,
			CreatedAt:         time.Now().UTC(),
		}, false, true
	}

	// Drain the events channel, capturing node outputs for node-scoped graders.
	// The channel is closed by the engine after UpdateRunStatus completes, so
	// store.GetRun called after this range loop will always see the final status.
	nodeOutputs := make(map[string]map[string]any)
	for event := range handle.Events {
		if event.Type == engine.EventNodeSucceeded && captureNodes[event.NodeID] {
			out := make(map[string]any, len(event.Output))
			for k, v := range event.Output {
				out[k] = v
			}
			nodeOutputs[event.NodeID] = out
		}
	}

	// Fetch the final run record (guaranteed updated before events channel closed).
	wfRun, err := r.store.GetRun(ctx, handle.RunID)
	if err != nil {
		// Return immediately — do not assume the workflow failed; the run may have
		// succeeded but the store fetch failed transiently. Reporting "workflow run
		// failed" when the run actually succeeded would produce misleading verdicts.
		explanation := fmt.Sprintf("could not fetch workflow run status: %v", err)
		slog.Error("eval runner: get run after drain", "eval_run_id", evalRunID, "run_id", handle.RunID, "error", err)
		return store.TestCaseResult{
			ID:                uuid.New().String(),
			EvalRunID:         evalRunID,
			TestCaseID:        tc.ID,
			TestCaseName:      tc.Name,
			WorkflowRunID:     handle.RunID,
			WorkflowRunStatus: string(store.RunStatusFailed),
			NodeOutputs:       nodeOutputs,
			GraderResults:     errorGraderResults(tc.Graders, explanation),
			Passed:            false,
			CreatedAt:         time.Now().UTC(),
		}, false, true
	}

	workflowRunStatus := string(wfRun.Status)
	finalOutput := wfRun.FinalOutput
	runFailed := workflowRunStatus == string(store.RunStatusFailed)

	// Evaluate graders.
	graderResults := make([]store.GraderResult, 0, len(tc.Graders))
	passCount := 0
	for _, g := range tc.Graders {
		var data map[string]any
		switch g.Scope {
		case "node":
			if nodeOut, ok := nodeOutputs[g.NodeID]; ok {
				data = nodeOut
			} else {
				msg := "node did not execute"
				if runFailed {
					msg = "node did not execute (workflow run failed)"
				}
				graderResults = append(graderResults, store.GraderResult{
					GraderID:    g.ID,
					GraderName:  g.Name,
					GraderType:  g.Type,
					Verdict:     store.VerdictError,
					Explanation: msg,
				})
				continue
			}
		default: // "workflow"
			if runFailed {
				graderResults = append(graderResults, store.GraderResult{
					GraderID:    g.ID,
					GraderName:  g.Name,
					GraderType:  g.Type,
					Verdict:     store.VerdictError,
					Explanation: "workflow run failed",
				})
				continue
			}
			data = finalOutput
		}

		grader, err := BuildGrader(g, r.llmFactory)
		if err != nil {
			graderResults = append(graderResults, store.GraderResult{
				GraderID:    g.ID,
				GraderName:  g.Name,
				GraderType:  g.Type,
				Verdict:     store.VerdictError,
				Explanation: "grader not available: " + err.Error(),
			})
			continue
		}

		gr := grader.Grade(ctx, data)
		gr.GraderID = g.ID
		gr.GraderName = g.Name
		gr.GraderType = g.Type
		if gr.Verdict == store.VerdictPass {
			passCount++
		}
		graderResults = append(graderResults, gr)
	}

	// Compute pass/fail status.
	isErr := runFailed
	passed := false
	if !isErr {
		if len(tc.Graders) == 0 {
			passed = true // smoke test: workflow completed without error
		} else {
			passRate := float64(passCount) / float64(len(tc.Graders))
			passed = passRate >= suite.PassThreshold
		}
	}

	result := store.TestCaseResult{
		ID:                uuid.New().String(),
		EvalRunID:         evalRunID,
		TestCaseID:        tc.ID,
		TestCaseName:      tc.Name,
		WorkflowRunID:     handle.RunID,
		WorkflowRunStatus: workflowRunStatus,
		NodeOutputs:       nodeOutputs,
		GraderResults:     graderResults,
		Passed:            passed,
		CreatedAt:         time.Now().UTC(),
	}
	return result, passed, isErr
}

// errorGraderResults returns error-verdict results for all graders in a slice.
func errorGraderResults(graders []store.GraderDef, explanation string) []store.GraderResult {
	results := make([]store.GraderResult, len(graders))
	for i, g := range graders {
		results[i] = store.GraderResult{
			GraderID:    g.ID,
			GraderName:  g.Name,
			GraderType:  g.Type,
			Verdict:     store.VerdictError,
			Explanation: explanation,
		}
	}
	return results
}
