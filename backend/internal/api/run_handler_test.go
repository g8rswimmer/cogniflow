package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/g8rswimmer/cogniflow/internal/store"
)

func setupRunHandler(t *testing.T) (*runHandler, *mockStore, *mockDispatcher) {
	t.Helper()
	ms := newMockStore()
	disp := &mockDispatcher{}
	return &runHandler{store: ms, dispatcher: disp}, ms, disp
}

// ---- triggerRun ----------------------------------------------------------

func TestRunHandler_TriggerRun_Success(t *testing.T) {
	h, ms, disp := setupRunHandler(t)

	ms.workflows["wf-1"] = store.Workflow{ID: "wf-1", Name: "Flow", Trigger: store.Trigger{Kind: "manual"}}
	disp.runID = "run-abc"
	ms.runs["run-abc"] = store.Run{ID: "run-abc", WorkflowID: "wf-1", Status: store.RunStatusRunning}

	r := httptest.NewRequest("POST", "/workflows/wf-1/runs", strings.NewReader(`{"initial_data":{"key":"val"}}`))
	r.SetPathValue("id", "wf-1")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.triggerRun(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["run_id"] != "run-abc" {
		t.Errorf("expected run_id=run-abc, got %v", resp["run_id"])
	}
}

func TestRunHandler_TriggerRun_WorkflowNotFound(t *testing.T) {
	h, _, _ := setupRunHandler(t)

	r := httptest.NewRequest("POST", "/workflows/missing/runs", nil)
	r.SetPathValue("id", "missing")
	w := httptest.NewRecorder()
	h.triggerRun(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
	assertErrorCode(t, w.Body.Bytes(), "NOT_FOUND")
}

func TestRunHandler_TriggerRun_EmptyBody(t *testing.T) {
	h, ms, disp := setupRunHandler(t)
	ms.workflows["wf-1"] = store.Workflow{ID: "wf-1", Trigger: store.Trigger{Kind: "manual"}}
	disp.runID = "run-1"
	ms.runs["run-1"] = store.Run{ID: "run-1", WorkflowID: "wf-1", Status: store.RunStatusRunning}

	r := httptest.NewRequest("POST", "/workflows/wf-1/runs", nil)
	r.SetPathValue("id", "wf-1")
	w := httptest.NewRecorder()
	h.triggerRun(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestRunHandler_TriggerRun_DispatchError(t *testing.T) {
	h, ms, disp := setupRunHandler(t)
	ms.workflows["wf-1"] = store.Workflow{ID: "wf-1", Trigger: store.Trigger{Kind: "manual"}}
	disp.dispErr = errInternal

	r := httptest.NewRequest("POST", "/workflows/wf-1/runs", nil)
	r.SetPathValue("id", "wf-1")
	w := httptest.NewRecorder()
	h.triggerRun(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
	assertErrorCode(t, w.Body.Bytes(), "ENGINE_ERROR")
}

// ---- getRun --------------------------------------------------------------

func TestRunHandler_GetRun_Found(t *testing.T) {
	h, ms, _ := setupRunHandler(t)
	now := time.Now().UTC()
	ms.runs["run-xyz"] = store.Run{
		ID:          "run-xyz",
		WorkflowID:  "wf-1",
		TriggeredBy: "manual",
		Status:      store.RunStatusSucceeded,
		StartedAt:   &now,
	}

	r := httptest.NewRequest("GET", "/runs/run-xyz", nil)
	r.SetPathValue("run_id", "run-xyz")
	w := httptest.NewRecorder()
	h.getRun(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["run_id"] != "run-xyz" {
		t.Errorf("expected run_id=run-xyz, got %v", resp["run_id"])
	}
	if resp["status"] != "succeeded" {
		t.Errorf("expected status=succeeded, got %v", resp["status"])
	}
}

func TestRunHandler_GetRun_NotFound(t *testing.T) {
	h, _, _ := setupRunHandler(t)

	r := httptest.NewRequest("GET", "/runs/nope", nil)
	r.SetPathValue("run_id", "nope")
	w := httptest.NewRecorder()
	h.getRun(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
	assertErrorCode(t, w.Body.Bytes(), "NOT_FOUND")
}

// ---- listRuns ------------------------------------------------------------

func TestRunHandler_ListRuns_Empty(t *testing.T) {
	h, _, _ := setupRunHandler(t)

	r := httptest.NewRequest("GET", "/workflows/wf-1/runs", nil)
	r.SetPathValue("id", "wf-1")
	w := httptest.NewRecorder()
	h.listRuns(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	runs, ok := resp["runs"].([]any)
	if !ok {
		t.Fatalf("expected runs array, got %T", resp["runs"])
	}
	if len(runs) != 0 {
		t.Errorf("expected empty runs, got %d", len(runs))
	}
}

func TestRunHandler_TriggerRun_GetRunError(t *testing.T) {
	h, ms, disp := setupRunHandler(t)
	ms.workflows["wf-1"] = store.Workflow{ID: "wf-1", Trigger: store.Trigger{Kind: "manual"}}
	disp.runID = "run-missing"
	ms.getRunErr = errInternal // run was created but GetRun fails

	r := httptest.NewRequest("POST", "/workflows/wf-1/runs", nil)
	r.SetPathValue("id", "wf-1")
	w := httptest.NewRecorder()
	h.triggerRun(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
	assertErrorCode(t, w.Body.Bytes(), "INTERNAL_ERROR")
}

func TestRunHandler_GetRun_InternalError(t *testing.T) {
	h, ms, _ := setupRunHandler(t)
	ms.getRunErr = errInternal

	r := httptest.NewRequest("GET", "/runs/any", nil)
	r.SetPathValue("run_id", "any")
	w := httptest.NewRecorder()
	h.getRun(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
	assertErrorCode(t, w.Body.Bytes(), "INTERNAL_ERROR")
}

func TestRunHandler_ListRuns_WithRuns(t *testing.T) {
	h, ms, _ := setupRunHandler(t)
	now := time.Now().UTC()
	ms.runs["r1"] = store.Run{ID: "r1", WorkflowID: "wf-1", Status: store.RunStatusSucceeded, StartedAt: &now}
	ms.runs["r2"] = store.Run{ID: "r2", WorkflowID: "wf-1", Status: store.RunStatusFailed, StartedAt: &now}
	ms.runs["r3"] = store.Run{ID: "r3", WorkflowID: "wf-other", Status: store.RunStatusRunning, StartedAt: &now}

	r := httptest.NewRequest("GET", "/workflows/wf-1/runs", nil)
	r.SetPathValue("id", "wf-1")
	w := httptest.NewRecorder()
	h.listRuns(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	runs := resp["runs"].([]any)
	if len(runs) != 2 {
		t.Errorf("expected 2 runs for wf-1, got %d", len(runs))
	}
}
