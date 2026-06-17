package eval

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/g8rswimmer/cogniflow/internal/store"
)

func TestCompareRuns(t *testing.T) {
	boolPtr := func(b bool) *bool { return &b }
	now := time.Now()

	makeRun := func(id, suiteID string, status store.EvalRunStatus) store.EvalRun {
		return store.EvalRun{
			ID:      id,
			SuiteID: suiteID,
			Status:  status,
		}
	}
	makeResult := func(id, runID, caseID, caseName string, passed bool) store.TestCaseResult {
		return store.TestCaseResult{
			ID:           id,
			EvalRunID:    runID,
			TestCaseID:   caseID,
			TestCaseName: caseName,
			Passed:       passed,
			CreatedAt:    now,
		}
	}

	type tc struct {
		name           string
		setup          func(st *stubStore)
		headRunID      string
		baselineRunID  string
		wantStatus     int
		wantErrCode    string
		wantRegressed  int
		wantImproved   int
		wantUnchanged  int
		wantNewCase    int
		wantMissing    int
		checkFirstCase func(t *testing.T, c testCaseComparison)
	}

	tests := []tc{
		{
			name: "regression: head fails, baseline passed",
			setup: func(st *stubStore) {
				st.seedSuite(store.EvalSuite{ID: "s1", WorkflowID: "w1"})
				st.evalRuns["er-head"] = makeRun("er-head", "s1", store.EvalRunCompleted)
				st.evalRuns["er-base"] = makeRun("er-base", "s1", store.EvalRunCompleted)
				st.tcResults["r1"] = makeResult("r1", "er-head", "tc1", "Case A", false)
				st.tcResults["r2"] = makeResult("r2", "er-base", "tc1", "Case A", true)
			},
			headRunID: "er-head", baselineRunID: "er-base",
			wantStatus: http.StatusOK, wantRegressed: 1,
			checkFirstCase: func(t *testing.T, c testCaseComparison) {
				t.Helper()
				if c.ChangeType != changeTypeRegressed {
					t.Errorf("want regressed, got %s", c.ChangeType)
				}
				if *c.HeadPassed != false {
					t.Errorf("want head_passed=false")
				}
				if *c.BaselinePassed != true {
					t.Errorf("want baseline_passed=true")
				}
			},
		},
		{
			name: "improvement: head passes, baseline failed",
			setup: func(st *stubStore) {
				st.seedSuite(store.EvalSuite{ID: "s1", WorkflowID: "w1"})
				st.evalRuns["er-head"] = makeRun("er-head", "s1", store.EvalRunCompleted)
				st.evalRuns["er-base"] = makeRun("er-base", "s1", store.EvalRunCompleted)
				st.tcResults["r1"] = makeResult("r1", "er-head", "tc1", "Case A", true)
				st.tcResults["r2"] = makeResult("r2", "er-base", "tc1", "Case A", false)
			},
			headRunID: "er-head", baselineRunID: "er-base",
			wantStatus: http.StatusOK, wantImproved: 1,
		},
		{
			name: "unchanged: same pass result in both",
			setup: func(st *stubStore) {
				st.seedSuite(store.EvalSuite{ID: "s1", WorkflowID: "w1"})
				st.evalRuns["er-head"] = makeRun("er-head", "s1", store.EvalRunCompleted)
				st.evalRuns["er-base"] = makeRun("er-base", "s1", store.EvalRunCompleted)
				st.tcResults["r1"] = makeResult("r1", "er-head", "tc1", "Case A", true)
				st.tcResults["r2"] = makeResult("r2", "er-base", "tc1", "Case A", true)
			},
			headRunID: "er-head", baselineRunID: "er-base",
			wantStatus: http.StatusOK, wantUnchanged: 1,
		},
		{
			name: "new case: present in head only",
			setup: func(st *stubStore) {
				st.seedSuite(store.EvalSuite{ID: "s1", WorkflowID: "w1"})
				st.evalRuns["er-head"] = makeRun("er-head", "s1", store.EvalRunCompleted)
				st.evalRuns["er-base"] = makeRun("er-base", "s1", store.EvalRunCompleted)
				st.tcResults["r1"] = makeResult("r1", "er-head", "tc1", "Case A", true)
			},
			headRunID: "er-head", baselineRunID: "er-base",
			wantStatus: http.StatusOK, wantNewCase: 1,
			checkFirstCase: func(t *testing.T, c testCaseComparison) {
				t.Helper()
				if c.ChangeType != changeTypeNewCase {
					t.Errorf("want new_case, got %s", c.ChangeType)
				}
				if c.HeadPassed == nil {
					t.Errorf("want head_passed non-nil")
				}
				if c.BaselinePassed != nil {
					t.Errorf("want baseline_passed nil, got %v", *c.BaselinePassed)
				}
			},
		},
		{
			name: "missing case: present in baseline only",
			setup: func(st *stubStore) {
				st.seedSuite(store.EvalSuite{ID: "s1", WorkflowID: "w1"})
				st.evalRuns["er-head"] = makeRun("er-head", "s1", store.EvalRunCompleted)
				st.evalRuns["er-base"] = makeRun("er-base", "s1", store.EvalRunCompleted)
				st.tcResults["r2"] = makeResult("r2", "er-base", "tc1", "Case A", false)
			},
			headRunID: "er-head", baselineRunID: "er-base",
			wantStatus: http.StatusOK, wantMissing: 1,
			checkFirstCase: func(t *testing.T, c testCaseComparison) {
				t.Helper()
				if c.ChangeType != changeTypeMissing {
					t.Errorf("want missing, got %s", c.ChangeType)
				}
				if c.HeadPassed != nil {
					t.Errorf("want head_passed nil, got %v", *c.HeadPassed)
				}
				if c.BaselinePassed == nil {
					t.Errorf("want baseline_passed non-nil")
				}
			},
		},
		{
			name: "mixed: regression + improvement + new + missing + unchanged",
			setup: func(st *stubStore) {
				st.seedSuite(store.EvalSuite{ID: "s1", WorkflowID: "w1"})
				st.evalRuns["er-head"] = makeRun("er-head", "s1", store.EvalRunCompleted)
				st.evalRuns["er-base"] = makeRun("er-base", "s1", store.EvalRunCompleted)
				// regressed
				st.tcResults["r1h"] = makeResult("r1h", "er-head", "tc1", "Case Regressed", false)
				st.tcResults["r1b"] = makeResult("r1b", "er-base", "tc1", "Case Regressed", true)
				// improved
				st.tcResults["r2h"] = makeResult("r2h", "er-head", "tc2", "Case Improved", true)
				st.tcResults["r2b"] = makeResult("r2b", "er-base", "tc2", "Case Improved", false)
				// unchanged
				st.tcResults["r3h"] = makeResult("r3h", "er-head", "tc3", "Case Unchanged", true)
				st.tcResults["r3b"] = makeResult("r3b", "er-base", "tc3", "Case Unchanged", true)
				// new
				st.tcResults["r4h"] = makeResult("r4h", "er-head", "tc4", "Case New", true)
				// missing
				st.tcResults["r5b"] = makeResult("r5b", "er-base", "tc5", "Case Missing", false)
			},
			headRunID: "er-head", baselineRunID: "er-base",
			wantStatus:    http.StatusOK,
			wantRegressed: 1,
			wantImproved:  1,
			wantUnchanged: 1,
			wantNewCase:   1,
			wantMissing:   1,
		},
		{
			name:          "missing baseline_run_id param",
			setup:         func(st *stubStore) {},
			headRunID:     "er-head",
			baselineRunID: "",
			wantStatus:    http.StatusBadRequest,
			wantErrCode:   "VALIDATION_FAILED",
		},
		{
			name:          "same run ID",
			setup:         func(st *stubStore) {},
			headRunID:     "er-1",
			baselineRunID: "er-1",
			wantStatus:    http.StatusBadRequest,
			wantErrCode:   "VALIDATION_FAILED",
		},
		{
			name:          "head run not found",
			setup:         func(st *stubStore) {},
			headRunID:     "er-missing",
			baselineRunID: "er-base",
			wantStatus:    http.StatusNotFound,
			wantErrCode:   "NOT_FOUND",
		},
		{
			name: "baseline run not found",
			setup: func(st *stubStore) {
				st.evalRuns["er-head"] = makeRun("er-head", "s1", store.EvalRunCompleted)
			},
			headRunID:     "er-head",
			baselineRunID: "er-missing",
			wantStatus:    http.StatusNotFound,
			wantErrCode:   "NOT_FOUND",
		},
		{
			name: "different suites",
			setup: func(st *stubStore) {
				st.evalRuns["er-head"] = makeRun("er-head", "s1", store.EvalRunCompleted)
				st.evalRuns["er-base"] = makeRun("er-base", "s2", store.EvalRunCompleted)
			},
			headRunID:     "er-head",
			baselineRunID: "er-base",
			wantStatus:    http.StatusBadRequest,
			wantErrCode:   "VALIDATION_FAILED",
		},
		{
			name: "head run not completed",
			setup: func(st *stubStore) {
				st.evalRuns["er-head"] = makeRun("er-head", "s1", store.EvalRunRunning)
				st.evalRuns["er-base"] = makeRun("er-base", "s1", store.EvalRunCompleted)
			},
			headRunID:     "er-head",
			baselineRunID: "er-base",
			wantStatus:    http.StatusBadRequest,
			wantErrCode:   "VALIDATION_FAILED",
		},
		{
			name: "baseline run not completed",
			setup: func(st *stubStore) {
				st.evalRuns["er-head"] = makeRun("er-head", "s1", store.EvalRunCompleted)
				st.evalRuns["er-base"] = makeRun("er-base", "s1", store.EvalRunPending)
			},
			headRunID:     "er-head",
			baselineRunID: "er-base",
			wantStatus:    http.StatusBadRequest,
			wantErrCode:   "VALIDATION_FAILED",
		},
	}

	_ = boolPtr
	_ = fmt.Sprintf

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, st := newTestHandler(t)
			tt.setup(st)

			path := "/v1/eval-runs/" + tt.headRunID + "/compare"
			if tt.baselineRunID != "" {
				path += "?baseline_run_id=" + tt.baselineRunID
			}
			rr := callHandler(h.CompareRuns, http.MethodGet, path, "", map[string]string{
				"eval_run_id": tt.headRunID,
			})

			if rr.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body: %s)", rr.Code, tt.wantStatus, rr.Body.String())
			}

			if tt.wantErrCode != "" {
				var resp struct {
					Error struct{ Code string } `json:"error"`
				}
				if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
					t.Fatalf("decode error response: %v", err)
				}
				if resp.Error.Code != tt.wantErrCode {
					t.Errorf("error code = %q, want %q", resp.Error.Code, tt.wantErrCode)
				}
				return
			}

			var resp evalRunCompareResponse
			if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode response: %v", err)
			}

			if resp.RegressedCount != tt.wantRegressed {
				t.Errorf("regressed_count = %d, want %d", resp.RegressedCount, tt.wantRegressed)
			}
			if resp.ImprovedCount != tt.wantImproved {
				t.Errorf("improved_count = %d, want %d", resp.ImprovedCount, tt.wantImproved)
			}
			if resp.UnchangedCount != tt.wantUnchanged {
				t.Errorf("unchanged_count = %d, want %d", resp.UnchangedCount, tt.wantUnchanged)
			}
			if resp.NewCaseCount != tt.wantNewCase {
				t.Errorf("new_case_count = %d, want %d", resp.NewCaseCount, tt.wantNewCase)
			}
			if resp.MissingCount != tt.wantMissing {
				t.Errorf("missing_count = %d, want %d", resp.MissingCount, tt.wantMissing)
			}

			if tt.checkFirstCase != nil && len(resp.Cases) > 0 {
				tt.checkFirstCase(t, resp.Cases[0])
			}
		})
	}
}
