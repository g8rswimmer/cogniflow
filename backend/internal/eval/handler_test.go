package eval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/g8rswimmer/cogniflow/internal/crypto"
	"github.com/g8rswimmer/cogniflow/internal/node"
	"github.com/g8rswimmer/cogniflow/internal/store"
)

// ---- test helpers ----------------------------------------------------------

func newTestHandler(t *testing.T) (*Handler, *stubStore) {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	c, err := crypto.NewCipher(key)
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	v := NewGraderVault(c)
	st := newStubStore()
	h := NewHandler(st, v, node.NewRegistry(), nil, nil)
	return h, st
}

func decodeJSON(t *testing.T, body []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(body, v); err != nil {
		t.Fatalf("decode response: %v (body: %s)", err, string(body))
	}
}

// callHandler calls a handler method with the given request and returns the response recorder.
func callHandler(h func(http.ResponseWriter, *http.Request), method, path, body string, pathValues map[string]string) *httptest.ResponseRecorder {
	return callHandlerH(h, method, path, body, pathValues, nil)
}

// callHandlerH is like callHandler but also sets extra HTTP headers.
func callHandlerH(h func(http.ResponseWriter, *http.Request), method, path, body string, pathValues, headers map[string]string) *httptest.ResponseRecorder {
	var buf *bytes.Reader
	if body != "" {
		buf = bytes.NewReader([]byte(body))
	} else {
		buf = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, buf)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range pathValues {
		req.SetPathValue(k, v)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	h(rr, req)
	return rr
}

// ---- stub store ------------------------------------------------------------

type stubStore struct {
	mu         sync.RWMutex
	suites     map[string]store.EvalSuite
	testCases  map[string]store.TestCase
	evalRuns   map[string]store.EvalRun
	tcResults  map[string]store.TestCaseResult
	workflows  map[string]store.Workflow

	createSuiteErr error
	getSuiteErr    error
	createCaseErr  error
}

func newStubStore() *stubStore {
	return &stubStore{
		suites:    make(map[string]store.EvalSuite),
		testCases: make(map[string]store.TestCase),
		evalRuns:  make(map[string]store.EvalRun),
		tcResults: make(map[string]store.TestCaseResult),
		workflows: make(map[string]store.Workflow),
	}
}

// seed helpers

func (s *stubStore) seedWorkflow(wf store.Workflow) {
	s.mu.Lock()
	s.workflows[wf.ID] = wf
	s.mu.Unlock()
}

func (s *stubStore) seedSuite(suite store.EvalSuite) {
	s.mu.Lock()
	s.suites[suite.ID] = suite
	s.mu.Unlock()
}

func (s *stubStore) seedCase(tc store.TestCase) {
	s.mu.Lock()
	s.testCases[tc.ID] = tc
	s.mu.Unlock()
}

// store.Store implementations

func (s *stubStore) CreateEvalSuite(_ context.Context, suite store.EvalSuite) (store.EvalSuite, error) {
	if s.createSuiteErr != nil {
		return store.EvalSuite{}, s.createSuiteErr
	}
	if suite.ID == "" {
		suite.ID = fmt.Sprintf("es-%d", time.Now().UnixNano())
	}
	suite.CreatedAt = time.Now()
	suite.UpdatedAt = suite.CreatedAt
	s.mu.Lock()
	s.suites[suite.ID] = suite
	s.mu.Unlock()
	return suite, nil
}

func (s *stubStore) GetEvalSuite(_ context.Context, id string) (store.EvalSuite, error) {
	if s.getSuiteErr != nil {
		return store.EvalSuite{}, s.getSuiteErr
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	suite, ok := s.suites[id]
	if !ok {
		return store.EvalSuite{}, store.ErrNotFound
	}
	return suite, nil
}

func (s *stubStore) ListEvalSuites(_ context.Context, workflowID string) ([]store.EvalSuiteSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []store.EvalSuiteSummary
	for _, su := range s.suites {
		if su.WorkflowID == workflowID {
			out = append(out, store.EvalSuiteSummary{EvalSuite: su})
		}
	}
	return out, nil
}

func (s *stubStore) ListEvalSuitesByCronTrigger(_ context.Context) ([]store.EvalSuite, error) {
	return nil, nil
}

func (s *stubStore) UpdateEvalSuite(_ context.Context, suite store.EvalSuite) (store.EvalSuite, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.suites[suite.ID]; !ok {
		return store.EvalSuite{}, store.ErrNotFound
	}
	s.suites[suite.ID] = suite
	return suite, nil
}

func (s *stubStore) DeleteEvalSuite(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.suites[id]; !ok {
		return store.ErrNotFound
	}
	delete(s.suites, id)
	return nil
}

func (s *stubStore) CreateTestCase(_ context.Context, tc store.TestCase) (store.TestCase, error) {
	if s.createCaseErr != nil {
		return store.TestCase{}, s.createCaseErr
	}
	if tc.ID == "" {
		tc.ID = fmt.Sprintf("tc-%d", time.Now().UnixNano())
	}
	tc.CreatedAt = time.Now()
	tc.UpdatedAt = tc.CreatedAt
	s.mu.Lock()
	s.testCases[tc.ID] = tc
	s.mu.Unlock()
	return tc, nil
}

func (s *stubStore) GetTestCase(_ context.Context, id string) (store.TestCase, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tc, ok := s.testCases[id]
	if !ok {
		return store.TestCase{}, store.ErrNotFound
	}
	return tc, nil
}

func (s *stubStore) ListTestCases(_ context.Context, suiteID string) ([]store.TestCase, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []store.TestCase
	for _, tc := range s.testCases {
		if tc.SuiteID == suiteID {
			out = append(out, tc)
		}
	}
	return out, nil
}

func (s *stubStore) UpdateTestCase(_ context.Context, tc store.TestCase) (store.TestCase, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.testCases[tc.ID]; !ok {
		return store.TestCase{}, store.ErrNotFound
	}
	s.testCases[tc.ID] = tc
	return tc, nil
}

func (s *stubStore) DeleteTestCase(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.testCases[id]; !ok {
		return store.ErrNotFound
	}
	delete(s.testCases, id)
	return nil
}

func (s *stubStore) ReorderTestCases(_ context.Context, suiteID string, ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, id := range ids {
		tc, ok := s.testCases[id]
		if !ok || tc.SuiteID != suiteID {
			return store.ErrNotFound
		}
		tc.Position = i
		s.testCases[id] = tc
	}
	return nil
}

func (s *stubStore) CreateEvalRun(_ context.Context, r store.EvalRun) (store.EvalRun, error) {
	if r.ID == "" {
		r.ID = fmt.Sprintf("er-%d", time.Now().UnixNano())
	}
	r.CreatedAt = time.Now()
	s.mu.Lock()
	s.evalRuns[r.ID] = r
	s.mu.Unlock()
	return r, nil
}

func (s *stubStore) GetEvalRun(_ context.Context, id string) (store.EvalRun, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.evalRuns[id]
	if !ok {
		return store.EvalRun{}, store.ErrNotFound
	}
	return r, nil
}

func (s *stubStore) ListEvalRuns(_ context.Context, f store.EvalRunFilter) ([]store.EvalRun, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []store.EvalRun
	for _, r := range s.evalRuns {
		if r.SuiteID == f.SuiteID {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *stubStore) UpdateEvalRunStatus(_ context.Context, id string, status store.EvalRunStatus, _ store.EvalRunCounts) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.evalRuns[id]
	if !ok {
		return store.ErrNotFound
	}
	r.Status = status
	s.evalRuns[id] = r
	return nil
}

func (s *stubStore) IncrementEvalRunCounts(_ context.Context, _ string, _ store.EvalRunCounts) error {
	return nil
}

func (s *stubStore) CreateTestCaseResult(_ context.Context, r store.TestCaseResult) (store.TestCaseResult, error) {
	if r.ID == "" {
		r.ID = fmt.Sprintf("tcr-%d", time.Now().UnixNano())
	}
	r.CreatedAt = time.Now()
	s.mu.Lock()
	s.tcResults[r.ID] = r
	s.mu.Unlock()
	return r, nil
}

func (s *stubStore) GetTestCaseResult(_ context.Context, id string) (store.TestCaseResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.tcResults[id]
	if !ok {
		return store.TestCaseResult{}, store.ErrNotFound
	}
	return r, nil
}

func (s *stubStore) ListTestCaseResults(_ context.Context, evalRunID string) ([]store.TestCaseResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []store.TestCaseResult
	for _, r := range s.tcResults {
		if r.EvalRunID == evalRunID {
			out = append(out, r)
		}
	}
	return out, nil
}

// remaining store.Store methods — not used in eval handler but required by interface

func (s *stubStore) CreateWorkflow(_ context.Context, w store.Workflow) (store.Workflow, error) {
	return w, nil
}
func (s *stubStore) GetWorkflow(_ context.Context, id string) (store.Workflow, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	wf, ok := s.workflows[id]
	if !ok {
		return store.Workflow{}, store.ErrNotFound
	}
	return wf, nil
}
func (s *stubStore) GetWorkflowSchema(_ context.Context, _ string) (json.RawMessage, error) {
	return nil, nil
}
func (s *stubStore) ListWorkflows(_ context.Context) ([]store.WorkflowSummary, error) { return nil, nil }
func (s *stubStore) UpdateWorkflow(_ context.Context, w store.Workflow) (store.Workflow, error) {
	return w, nil
}
func (s *stubStore) DeleteWorkflow(_ context.Context, _ string) error { return nil }
func (s *stubStore) CreateRun(_ context.Context, r store.Run) (store.Run, error)    { return r, nil }
func (s *stubStore) UpdateRunStatus(_ context.Context, _ string, _ store.RunStatus, _ map[string]any) error {
	return nil
}
func (s *stubStore) SaveRunNodeResults(_ context.Context, _ string, _ map[string]store.NodeResult) error {
	return nil
}
func (s *stubStore) GetRun(_ context.Context, _ string) (store.Run, error) {
	return store.Run{}, store.ErrNotFound
}
func (s *stubStore) ListRuns(_ context.Context, _ store.RunFilter) ([]store.Run, error) {
	return nil, nil
}
func (s *stubStore) UpsertChunks(_ context.Context, _ []store.RAGChunk) error { return nil }
func (s *stubStore) SearchChunks(_ context.Context, _ []float32, _ int, _ string) ([]store.RAGChunkResult, error) {
	return nil, nil
}
func (s *stubStore) SaveTriggerConfig(_ context.Context, _ string, _ store.TriggerConfig) error {
	return nil
}
func (s *stubStore) GetTriggerConfig(_ context.Context, _ string) (store.TriggerConfig, error) {
	return store.TriggerConfig{}, nil
}
func (s *stubStore) ListTriggerConfigs(_ context.Context) ([]store.WorkflowTrigger, error) {
	return nil, nil
}
func (s *stubStore) SavePluginRegistration(_ context.Context, _ store.PluginRegistration) error {
	return nil
}
func (s *stubStore) GetPluginRegistration(_ context.Context, _ string) (store.PluginRegistration, error) {
	return store.PluginRegistration{}, store.ErrNotFound
}
func (s *stubStore) ListPluginRegistrations(_ context.Context) ([]store.PluginRegistration, error) {
	return nil, nil
}
func (s *stubStore) DeletePluginRegistration(_ context.Context, _ string) error { return nil }

// ---- Suite handler tests ---------------------------------------------------

func TestHandler_CreateSuite(t *testing.T) {
	h, _ := newTestHandler(t)
	body := `{"name":"Smoke Suite","pass_threshold":0.8,"max_concurrency":2}`
	rr := callHandler(h.CreateSuite, "POST", "/v1/workflows/wf-1/eval-suites", body,
		map[string]string{"workflow_id": "wf-1"})

	if rr.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp store.EvalSuite
	decodeJSON(t, rr.Body.Bytes(), &resp)
	if resp.ID == "" {
		t.Error("expected id in response")
	}
	if resp.Name != "Smoke Suite" {
		t.Errorf("name: got %q", resp.Name)
	}
	if resp.PassThreshold != 0.8 {
		t.Errorf("pass_threshold: got %f", resp.PassThreshold)
	}
}

func TestHandler_CreateSuite_MissingName(t *testing.T) {
	h, _ := newTestHandler(t)
	rr := callHandler(h.CreateSuite, "POST", "/v1/workflows/wf-1/eval-suites", `{}`,
		map[string]string{"workflow_id": "wf-1"})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rr.Code)
	}
	var resp map[string]any
	decodeJSON(t, rr.Body.Bytes(), &resp)
	errObj := resp["error"].(map[string]any)
	if errObj["code"] != "VALIDATION_FAILED" {
		t.Errorf("code: got %q", errObj["code"])
	}
}

func TestHandler_GetSuite(t *testing.T) {
	h, st := newTestHandler(t)
	st.seedSuite(store.EvalSuite{ID: "es-1", WorkflowID: "wf-1", Name: "S", PassThreshold: 1.0, MaxConcurrency: 1})

	rr := callHandler(h.GetSuite, "GET", "/v1/eval-suites/es-1", "",
		map[string]string{"suite_id": "es-1"})
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	decodeJSON(t, rr.Body.Bytes(), &resp)
	if resp["name"] != "S" {
		t.Errorf("name: got %v", resp["name"])
	}
}

func TestHandler_GetSuite_NotFound(t *testing.T) {
	h, _ := newTestHandler(t)
	rr := callHandler(h.GetSuite, "GET", "/v1/eval-suites/ghost", "",
		map[string]string{"suite_id": "ghost"})
	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rr.Code)
	}
}

func TestHandler_UpdateSuite(t *testing.T) {
	h, st := newTestHandler(t)
	st.seedSuite(store.EvalSuite{ID: "es-1", WorkflowID: "wf-1", Name: "Old", PassThreshold: 1.0, MaxConcurrency: 1})

	body := `{"name":"New","pass_threshold":0.7}`
	rr := callHandler(h.UpdateSuite, "PUT", "/v1/eval-suites/es-1", body,
		map[string]string{"suite_id": "es-1"})
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp store.EvalSuite
	decodeJSON(t, rr.Body.Bytes(), &resp)
	if resp.Name != "New" {
		t.Errorf("name: got %q", resp.Name)
	}
}

func TestHandler_DeleteSuite(t *testing.T) {
	h, st := newTestHandler(t)
	st.seedSuite(store.EvalSuite{ID: "es-1", WorkflowID: "wf-1", Name: "S"})

	rr := callHandler(h.DeleteSuite, "DELETE", "/v1/eval-suites/es-1", "",
		map[string]string{"suite_id": "es-1"})
	if rr.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandler_DeleteSuite_NotFound(t *testing.T) {
	h, _ := newTestHandler(t)
	rr := callHandler(h.DeleteSuite, "DELETE", "/v1/eval-suites/ghost", "",
		map[string]string{"suite_id": "ghost"})
	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rr.Code)
	}
}

// ---- Test case handler tests -----------------------------------------------

func TestHandler_CreateCase_HappyPath(t *testing.T) {
	h, st := newTestHandler(t)
	st.seedSuite(store.EvalSuite{ID: "es-1", WorkflowID: "wf-1", Name: "S"})
	// Seed workflow with a node so mock validation passes.
	st.seedWorkflow(store.Workflow{
		ID:    "wf-1",
		Name:  "W",
		Nodes: []store.WorkflowNode{{ID: "n1", TypeID: "http.request"}},
	})

	body := `{
		"name": "Happy path",
		"initial_data": {"ticket": "billing"},
		"mocks": [{"node_id": "n1", "output": {"status": 200}}],
		"graders": [
			{"id":"g1","name":"check","type":"string_match","scope":"workflow",
			 "config":{"field_path":"completion","match_type":"contains","expected_value":"ok"}}
		]
	}`
	rr := callHandler(h.CreateCase, "POST", "/v1/eval-suites/es-1/test-cases", body,
		map[string]string{"suite_id": "es-1"})
	if rr.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp store.TestCase
	decodeJSON(t, rr.Body.Bytes(), &resp)
	if resp.Name != "Happy path" {
		t.Errorf("name: got %q", resp.Name)
	}
}

func TestHandler_CreateCase_LLMJudgeApiKeyMasked(t *testing.T) {
	h, st := newTestHandler(t)
	st.seedSuite(store.EvalSuite{ID: "es-1", WorkflowID: "wf-1", Name: "S"})

	body := `{
		"name": "LLM check",
		"initial_data": {},
		"mocks": [],
		"graders": [{
			"id":"g1","name":"quality","type":"llm_judge","scope":"workflow",
			"config":{"provider":"openai","model":"gpt-4o","api_key":"sk-real-key","rubric":"Is it helpful?"}
		}]
	}`
	rr := callHandler(h.CreateCase, "POST", "/v1/eval-suites/es-1/test-cases", body,
		map[string]string{"suite_id": "es-1"})
	if rr.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	decodeJSON(t, rr.Body.Bytes(), &resp)

	graders, _ := resp["graders"].([]any)
	if len(graders) == 0 {
		t.Fatal("expected graders in response")
	}
	grader := graders[0].(map[string]any)
	config := grader["config"].(map[string]any)
	if config["api_key"] != "***" {
		t.Errorf("api_key should be masked in response, got %q", config["api_key"])
	}
}

func TestHandler_CreateCase_InvalidMockNodeID(t *testing.T) {
	h, st := newTestHandler(t)
	st.seedSuite(store.EvalSuite{ID: "es-1", WorkflowID: "wf-1", Name: "S"})
	st.seedWorkflow(store.Workflow{ID: "wf-1", Name: "W", Nodes: []store.WorkflowNode{}})

	body := `{
		"name": "Bad mock",
		"initial_data": {},
		"mocks": [{"node_id": "does-not-exist", "output": {}}],
		"graders": []
	}`
	rr := callHandler(h.CreateCase, "POST", "/v1/eval-suites/es-1/test-cases", body,
		map[string]string{"suite_id": "es-1"})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	decodeJSON(t, rr.Body.Bytes(), &resp)
	errObj := resp["error"].(map[string]any)
	if errObj["code"] != "VALIDATION_FAILED" {
		t.Errorf("code: got %q", errObj["code"])
	}
}

func TestHandler_CreateCase_InvalidRegex(t *testing.T) {
	h, st := newTestHandler(t)
	st.seedSuite(store.EvalSuite{ID: "es-1", WorkflowID: "wf-1", Name: "S"})

	body := `{
		"name": "Bad regex",
		"initial_data": {},
		"mocks": [],
		"graders": [{
			"id":"g1","name":"r","type":"string_match","scope":"workflow",
			"config":{"field_path":"x","match_type":"regex","expected_value":"(broken"}
		}]
	}`
	rr := callHandler(h.CreateCase, "POST", "/v1/eval-suites/es-1/test-cases", body,
		map[string]string{"suite_id": "es-1"})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	decodeJSON(t, rr.Body.Bytes(), &resp)
	errObj := resp["error"].(map[string]any)
	if errObj["code"] != "VALIDATION_FAILED" {
		t.Errorf("code: got %q", errObj["code"])
	}
}

func TestHandler_CreateCase_LLMJudge_MissingRubric(t *testing.T) {
	h, st := newTestHandler(t)
	st.seedSuite(store.EvalSuite{ID: "es-1", WorkflowID: "wf-1", Name: "S"})

	body := `{
		"name": "No rubric",
		"initial_data": {},
		"mocks": [],
		"graders": [{
			"id":"g1","name":"judge","type":"llm_judge","scope":"workflow",
			"config":{"provider":"anthropic","model":"claude-haiku-4-5-20251001","api_key":"sk-ant-x"}
		}]
	}`
	rr := callHandler(h.CreateCase, "POST", "/v1/eval-suites/es-1/test-cases", body,
		map[string]string{"suite_id": "es-1"})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for missing rubric, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	decodeJSON(t, rr.Body.Bytes(), &resp)
	errObj := resp["error"].(map[string]any)
	if errObj["code"] != "VALIDATION_FAILED" {
		t.Errorf("code: got %q", errObj["code"])
	}
}

func TestHandler_CreateCase_LLMJudge_InvalidProvider(t *testing.T) {
	h, st := newTestHandler(t)
	st.seedSuite(store.EvalSuite{ID: "es-1", WorkflowID: "wf-1", Name: "S"})

	body := `{
		"name": "Bad provider",
		"initial_data": {},
		"mocks": [],
		"graders": [{
			"id":"g1","name":"judge","type":"llm_judge","scope":"workflow",
			"config":{"provider":"azure","model":"gpt-4o","api_key":"key","rubric":"Is it good?"}
		}]
	}`
	rr := callHandler(h.CreateCase, "POST", "/v1/eval-suites/es-1/test-cases", body,
		map[string]string{"suite_id": "es-1"})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for unknown provider, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	decodeJSON(t, rr.Body.Bytes(), &resp)
	errObj := resp["error"].(map[string]any)
	if errObj["code"] != "VALIDATION_FAILED" {
		t.Errorf("code: got %q", errObj["code"])
	}
}

func TestHandler_CreateCase_Checklist_MissingCriteria(t *testing.T) {
	h, st := newTestHandler(t)
	st.seedSuite(store.EvalSuite{ID: "es-1", WorkflowID: "wf-1", Name: "S"})

	body := `{
		"name": "No criteria",
		"initial_data": {},
		"mocks": [],
		"graders": [{
			"id":"g1","name":"cl","type":"checklist","scope":"workflow",
			"config":{"provider":"anthropic","model":"claude-haiku-4-5-20251001","api_key":"sk-ant-x"}
		}]
	}`
	rr := callHandler(h.CreateCase, "POST", "/v1/eval-suites/es-1/test-cases", body,
		map[string]string{"suite_id": "es-1"})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for missing criteria, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	decodeJSON(t, rr.Body.Bytes(), &resp)
	errObj := resp["error"].(map[string]any)
	if errObj["code"] != "VALIDATION_FAILED" {
		t.Errorf("code: got %q", errObj["code"])
	}
}

func TestHandler_CreateCase_Checklist_EmptyCriteria(t *testing.T) {
	h, st := newTestHandler(t)
	st.seedSuite(store.EvalSuite{ID: "es-1", WorkflowID: "wf-1", Name: "S"})

	body := `{
		"name": "Empty criteria",
		"initial_data": {},
		"mocks": [],
		"graders": [{
			"id":"g1","name":"cl","type":"checklist","scope":"workflow",
			"config":{"provider":"anthropic","model":"claude-haiku-4-5-20251001","api_key":"sk-ant-x","criteria":[]}
		}]
	}`
	rr := callHandler(h.CreateCase, "POST", "/v1/eval-suites/es-1/test-cases", body,
		map[string]string{"suite_id": "es-1"})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for empty criteria, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandler_CreateCase_LLMJudge_ValidPassesValidation(t *testing.T) {
	h, st := newTestHandler(t)
	st.seedSuite(store.EvalSuite{ID: "es-1", WorkflowID: "wf-1", Name: "S"})

	body := `{
		"name": "Valid judge",
		"initial_data": {},
		"mocks": [],
		"graders": [{
			"id":"g1","name":"judge","type":"llm_judge","scope":"workflow",
			"config":{"provider":"anthropic","model":"claude-haiku-4-5-20251001","api_key":"sk-ant-x","rubric":"Is it helpful?"}
		}]
	}`
	rr := callHandler(h.CreateCase, "POST", "/v1/eval-suites/es-1/test-cases", body,
		map[string]string{"suite_id": "es-1"})
	if rr.Code != http.StatusCreated {
		t.Fatalf("want 201 for valid llm_judge, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandler_GetCase(t *testing.T) {
	h, st := newTestHandler(t)
	st.seedCase(store.TestCase{
		ID: "tc-1", SuiteID: "es-1", Name: "TC",
		Graders: []store.GraderDef{
			{ID: "g1", Type: "llm_judge", Config: map[string]any{"api_key": "enc:abc", "model": "gpt-4o"}},
		},
	})

	rr := callHandler(h.GetCase, "GET", "/v1/eval-suites/es-1/test-cases/tc-1", "",
		map[string]string{"suite_id": "es-1", "case_id": "tc-1"})
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	var resp map[string]any
	decodeJSON(t, rr.Body.Bytes(), &resp)

	graders, _ := resp["graders"].([]any)
	if len(graders) == 0 {
		t.Fatal("expected graders")
	}
	cfg := graders[0].(map[string]any)["config"].(map[string]any)
	if cfg["api_key"] != "***" {
		t.Errorf("api_key should be masked, got %q", cfg["api_key"])
	}
}

func TestHandler_DeleteCase(t *testing.T) {
	h, st := newTestHandler(t)
	st.seedCase(store.TestCase{ID: "tc-1", SuiteID: "es-1", Name: "TC"})

	rr := callHandler(h.DeleteCase, "DELETE", "/v1/eval-suites/es-1/test-cases/tc-1", "",
		map[string]string{"suite_id": "es-1", "case_id": "tc-1"})
	if rr.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d", rr.Code)
	}
}

func TestHandler_ReorderCases(t *testing.T) {
	h, st := newTestHandler(t)
	st.seedSuite(store.EvalSuite{ID: "es-1", WorkflowID: "wf-1", Name: "S"})
	st.seedCase(store.TestCase{ID: "tc-1", SuiteID: "es-1", Name: "A", Position: 0})
	st.seedCase(store.TestCase{ID: "tc-2", SuiteID: "es-1", Name: "B", Position: 1})

	body := `{"case_ids":["tc-2","tc-1"]}`
	rr := callHandler(h.ReorderCases, "PUT", "/v1/eval-suites/es-1/test-cases/order", body,
		map[string]string{"suite_id": "es-1"})
	if rr.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandler_ListCases(t *testing.T) {
	h, st := newTestHandler(t)
	st.seedSuite(store.EvalSuite{ID: "es-1", WorkflowID: "wf-1", Name: "S"})
	st.seedCase(store.TestCase{ID: "tc-1", SuiteID: "es-1", Name: "A"})
	st.seedCase(store.TestCase{ID: "tc-2", SuiteID: "es-1", Name: "B"})

	rr := callHandler(h.ListCases, "GET", "/v1/eval-suites/es-1/test-cases", "",
		map[string]string{"suite_id": "es-1"})
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	var resp map[string]any
	decodeJSON(t, rr.Body.Bytes(), &resp)
	cases, _ := resp["test_cases"].([]any)
	if len(cases) != 2 {
		t.Errorf("want 2 test cases, got %d", len(cases))
	}
}

func TestHandler_UpdateCase_PreservesEncryptedKey(t *testing.T) {
	h, st := newTestHandler(t)
	st.seedSuite(store.EvalSuite{ID: "es-1", WorkflowID: "wf-1", Name: "S"})

	// Store a test case with an encrypted api_key.
	encVal := "enc:dGVzdA=="
	st.seedCase(store.TestCase{
		ID: "tc-1", SuiteID: "es-1", Name: "Old",
		Graders: []store.GraderDef{
			{ID: "g1", Type: "llm_judge", Config: map[string]any{"api_key": encVal, "rubric": "x"}},
		},
	})

	// Client sends "***" for the api_key — should preserve existing encrypted value.
	body := `{
		"name": "Updated",
		"initial_data": {},
		"mocks": [],
		"graders": [{
			"id":"g1","name":"quality","type":"llm_judge","scope":"workflow",
			"config":{"provider":"openai","model":"gpt-4o","api_key":"***","rubric":"Is it helpful?"}
		}]
	}`
	rr := callHandler(h.UpdateCase, "PUT", "/v1/eval-suites/es-1/test-cases/tc-1", body,
		map[string]string{"suite_id": "es-1", "case_id": "tc-1"})
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify the stored grader still has the enc: value (not "***").
	stored, _ := st.GetTestCase(context.Background(), "tc-1")
	if len(stored.Graders) == 0 {
		t.Fatal("expected graders in stored case")
	}
	apiKey, _ := stored.Graders[0].Config["api_key"].(string)
	// The stored key should be enc: (preserved) or the original encrypted value.
	if apiKey == "***" {
		t.Error("stored api_key should not be the sentinel '***'")
	}
}

func TestHandler_TriggerRun_SuiteNotFound(t *testing.T) {
	h, _ := newTestHandler(t)
	h.runner = &stubRunner{err: store.ErrNotFound}
	rr := callHandler(h.TriggerRun, "POST", "/v1/eval-suites/missing/runs", "{}",
		map[string]string{"suite_id": "missing"})
	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rr.Code)
	}
}

func TestHandler_TriggerRun_Success(t *testing.T) {
	h, st := newTestHandler(t)
	st.mu.Lock()
	st.suites["es-1"] = store.EvalSuite{ID: "es-1", WorkflowID: "wf-1", Name: "Suite"}
	st.mu.Unlock()

	stub := &stubRunner{runID: "er-abc123"}
	h.runner = stub

	rr := callHandler(h.TriggerRun, "POST", "/v1/eval-suites/es-1/runs", "{}",
		map[string]string{"suite_id": "es-1"})
	if rr.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	decodeJSON(t, rr.Body.Bytes(), &resp)
	if resp["id"] != "er-abc123" {
		t.Errorf("want id=er-abc123, got %q", resp["id"])
	}
}

func TestHandler_TriggerRun_RunnerError(t *testing.T) {
	h, st := newTestHandler(t)
	st.mu.Lock()
	st.suites["es-1"] = store.EvalSuite{ID: "es-1", WorkflowID: "wf-1", Name: "Suite"}
	st.mu.Unlock()

	h.runner = &stubRunner{err: fmt.Errorf("runner failed")}

	rr := callHandler(h.TriggerRun, "POST", "/v1/eval-suites/es-1/runs", "{}",
		map[string]string{"suite_id": "es-1"})
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", rr.Code)
	}
}

// stubRunner is a test double for evalRunnerI.
type stubRunner struct {
	runID       string
	triggeredBy string // captured on Execute for assertion
	err         error
}

func (s *stubRunner) Execute(_ context.Context, _ string, triggeredBy string) (string, error) {
	s.triggeredBy = triggeredBy
	return s.runID, s.err
}

// ---- WebhookTrigger tests --------------------------------------------------

// webhookSuite creates a test suite with trigger_kind=webhook and a real
// encrypted secret. Returns the suite and the plain-text secret.
func webhookSuite(t *testing.T, h *Handler, st *stubStore) (store.EvalSuite, string) {
	t.Helper()
	const plainSecret = "supersecrettoken123"
	enc, err := h.vault.EncryptValue(plainSecret)
	if err != nil {
		t.Fatalf("EncryptValue: %v", err)
	}
	suite := store.EvalSuite{
		ID:            "wh-suite-1",
		WorkflowID:    "wf-1",
		Name:          "CI Gate",
		TriggerKind:   "webhook",
		WebhookSecret: enc,
	}
	st.mu.Lock()
	st.suites[suite.ID] = suite
	st.mu.Unlock()
	return suite, plainSecret
}

func TestHandler_WebhookTrigger_Success(t *testing.T) {
	h, st := newTestHandler(t)
	_, secret := webhookSuite(t, h, st)
	runner := &stubRunner{runID: "run-wh-1"}
	h.runner = runner

	rr := callHandlerH(h.WebhookTrigger, "POST", "/v1/eval-webhooks/wh-suite-1", "",
		map[string]string{"suite_id": "wh-suite-1"},
		map[string]string{"Authorization": "Bearer " + secret},
	)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("want 202, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	decodeJSON(t, rr.Body.Bytes(), &resp)
	if resp["eval_run_id"] != "run-wh-1" {
		t.Errorf("want eval_run_id=run-wh-1, got %q", resp["eval_run_id"])
	}
	if runner.triggeredBy != "webhook" {
		t.Errorf("want triggered_by=webhook, got %q", runner.triggeredBy)
	}
}

func TestHandler_WebhookTrigger_WrongToken(t *testing.T) {
	h, st := newTestHandler(t)
	webhookSuite(t, h, st)
	h.runner = &stubRunner{runID: "run-1"}

	rr := callHandlerH(h.WebhookTrigger, "POST", "/v1/eval-webhooks/wh-suite-1", "",
		map[string]string{"suite_id": "wh-suite-1"},
		map[string]string{"Authorization": "Bearer wrongtoken"},
	)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rr.Code)
	}
	var resp map[string]any
	decodeJSON(t, rr.Body.Bytes(), &resp)
	if code := resp["error"].(map[string]any)["code"]; code != "UNAUTHORIZED" {
		t.Errorf("want UNAUTHORIZED, got %v", code)
	}
}

func TestHandler_WebhookTrigger_MissingAuthHeader(t *testing.T) {
	h, st := newTestHandler(t)
	webhookSuite(t, h, st)

	rr := callHandler(h.WebhookTrigger, "POST", "/v1/eval-webhooks/wh-suite-1", "",
		map[string]string{"suite_id": "wh-suite-1"},
	)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rr.Code)
	}
}

func TestHandler_WebhookTrigger_SuiteNotFound(t *testing.T) {
	h, _ := newTestHandler(t)

	rr := callHandlerH(h.WebhookTrigger, "POST", "/v1/eval-webhooks/missing", "",
		map[string]string{"suite_id": "missing"},
		map[string]string{"Authorization": "Bearer token"},
	)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rr.Code)
	}
}

func TestHandler_WebhookTrigger_WrongTriggerKind(t *testing.T) {
	h, st := newTestHandler(t)
	st.mu.Lock()
	st.suites["cron-suite"] = store.EvalSuite{
		ID: "cron-suite", WorkflowID: "wf-1", Name: "Cron Suite", TriggerKind: "cron",
	}
	st.mu.Unlock()

	rr := callHandlerH(h.WebhookTrigger, "POST", "/v1/eval-webhooks/cron-suite", "",
		map[string]string{"suite_id": "cron-suite"},
		map[string]string{"Authorization": "Bearer token"},
	)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rr.Code)
	}
	var resp map[string]any
	decodeJSON(t, rr.Body.Bytes(), &resp)
	if code := resp["error"].(map[string]any)["code"]; code != "INVALID_TRIGGER" {
		t.Errorf("want INVALID_TRIGGER, got %v", code)
	}
}

func TestHandler_WebhookTrigger_WorkflowDeleted(t *testing.T) {
	h, st := newTestHandler(t)
	_, secret := webhookSuite(t, h, st)
	st.mu.Lock()
	s := st.suites["wh-suite-1"]
	s.WorkflowDeleted = true
	st.suites["wh-suite-1"] = s
	st.mu.Unlock()

	rr := callHandlerH(h.WebhookTrigger, "POST", "/v1/eval-webhooks/wh-suite-1", "",
		map[string]string{"suite_id": "wh-suite-1"},
		map[string]string{"Authorization": "Bearer " + secret},
	)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rr.Code)
	}
	var resp map[string]any
	decodeJSON(t, rr.Body.Bytes(), &resp)
	if code := resp["error"].(map[string]any)["code"]; code != "WORKFLOW_DELETED" {
		t.Errorf("want WORKFLOW_DELETED, got %v", code)
	}
}

// ---- ImportTestCases handler tests -----------------------------------------

// callImport builds a multipart/form-data request and calls ImportTestCases.
func callImport(h *Handler, suiteID, filename, content string) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", filename)
	_, _ = fw.Write([]byte(content))
	mw.Close()

	req := httptest.NewRequest("POST", "/v1/eval-suites/"+suiteID+"/test-cases/import", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.SetPathValue("suite_id", suiteID)

	rr := httptest.NewRecorder()
	h.ImportTestCases(rr, req)
	return rr
}

func TestHandler_ImportTestCases_SuiteNotFound(t *testing.T) {
	h, _ := newTestHandler(t)
	rr := callImport(h, "nonexistent", "data.csv", "name\nAlice\n")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandler_ImportTestCases_NoFileField(t *testing.T) {
	h, st := newTestHandler(t)
	st.seedSuite(store.EvalSuite{ID: "s1", WorkflowID: "wf1"})

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	// Write a field with the wrong name (not "file")
	_ = mw.WriteField("wrong_field", "value")
	mw.Close()

	req := httptest.NewRequest("POST", "/v1/eval-suites/s1/test-cases/import", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.SetPathValue("suite_id", "s1")

	rr := httptest.NewRecorder()
	h.ImportTestCases(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandler_ImportTestCases_UnsupportedExtension(t *testing.T) {
	h, st := newTestHandler(t)
	st.seedSuite(store.EvalSuite{ID: "s1", WorkflowID: "wf1"})

	rr := callImport(h, "s1", "data.txt", "some content")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandler_ImportTestCases_ValidCSV(t *testing.T) {
	h, st := newTestHandler(t)
	st.seedSuite(store.EvalSuite{ID: "s1", WorkflowID: "wf1"})

	csv := "name,description,city\nAlice,First,NYC\nBob,Second,LA\nCarol,Third,SF\n"
	rr := callImport(h, "s1", "data.csv", csv)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	decodeJSON(t, rr.Body.Bytes(), &resp)
	if got := int(resp["created"].(float64)); got != 3 {
		t.Errorf("created: want 3, got %d", got)
	}
	if got := int(resp["skipped"].(float64)); got != 0 {
		t.Errorf("skipped: want 0, got %d", got)
	}
	errs := resp["errors"].([]any)
	if len(errs) != 0 {
		t.Errorf("errors: want [], got %v", errs)
	}

	// Verify test cases were actually stored.
	st.mu.RLock()
	count := 0
	for _, tc := range st.testCases {
		if tc.SuiteID == "s1" {
			count++
		}
	}
	st.mu.RUnlock()
	if count != 3 {
		t.Errorf("stored test cases: want 3, got %d", count)
	}
}

func TestHandler_ImportTestCases_CSVWithBadRow(t *testing.T) {
	h, st := newTestHandler(t)
	st.seedSuite(store.EvalSuite{ID: "s1", WorkflowID: "wf1"})

	// Row 2 is valid, row 3 has empty name, row 4 is valid.
	csv := "name,description\nAlice,first\n,missing name\nBob,third\n"
	rr := callImport(h, "s1", "data.csv", csv)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	decodeJSON(t, rr.Body.Bytes(), &resp)
	if got := int(resp["created"].(float64)); got != 2 {
		t.Errorf("created: want 2, got %d", got)
	}
	if got := int(resp["skipped"].(float64)); got != 1 {
		t.Errorf("skipped: want 1, got %d", got)
	}
	errs := resp["errors"].([]any)
	if len(errs) != 1 {
		t.Fatalf("errors: want 1, got %d", len(errs))
	}
	errRow := errs[0].(map[string]any)
	if errRow["row"].(float64) != 3 {
		t.Errorf("error row: want 3, got %v", errRow["row"])
	}
}

func TestHandler_ImportTestCases_ValidJSONL(t *testing.T) {
	h, st := newTestHandler(t)
	st.seedSuite(store.EvalSuite{ID: "s2", WorkflowID: "wf1"})

	jsonl := `{"name":"Case A","description":"first","initial_data":{"x":1}}` + "\n" +
		`{"name":"Case B","initial_data":{"y":"hello"}}` + "\n"
	rr := callImport(h, "s2", "dataset.jsonl", jsonl)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	decodeJSON(t, rr.Body.Bytes(), &resp)
	if got := int(resp["created"].(float64)); got != 2 {
		t.Errorf("created: want 2, got %d", got)
	}
}

func TestHandler_ImportTestCases_RowLimitExceeded(t *testing.T) {
	h, st := newTestHandler(t)
	st.seedSuite(store.EvalSuite{ID: "s1", WorkflowID: "wf1"})

	// Build a CSV with 501 data rows.
	var sb strings.Builder
	sb.WriteString("name\n")
	for i := 0; i < 501; i++ {
		sb.WriteString("Row\n")
	}
	rr := callImport(h, "s1", "big.csv", sb.String())
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandler_ImportTestCases_HeaderOnlyCSV(t *testing.T) {
	h, st := newTestHandler(t)
	st.seedSuite(store.EvalSuite{ID: "s1", WorkflowID: "wf1"})

	rr := callImport(h, "s1", "empty.csv", "name,description\n")
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	decodeJSON(t, rr.Body.Bytes(), &resp)
	if got := int(resp["created"].(float64)); got != 0 {
		t.Errorf("created: want 0, got %d", got)
	}
}

func TestHandler_ImportTestCases_StoreError(t *testing.T) {
	h, st := newTestHandler(t)
	st.seedSuite(store.EvalSuite{ID: "s1", WorkflowID: "wf1"})
	st.createCaseErr = fmt.Errorf("simulated DB error")

	csv := "name\nAlice\nBob\n"
	rr := callImport(h, "s1", "data.csv", csv)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	decodeJSON(t, rr.Body.Bytes(), &resp)
	// All rows fail due to store error; created=0, skipped=2.
	if got := int(resp["created"].(float64)); got != 0 {
		t.Errorf("created: want 0, got %d", got)
	}
	if got := int(resp["skipped"].(float64)); got != 2 {
		t.Errorf("skipped: want 2, got %d", got)
	}
}
