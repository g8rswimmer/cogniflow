package trigger

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/g8rswimmer/cogniflow/internal/store"
)

// ---- test doubles -----------------------------------------------------------

// mockConfigGetter satisfies triggerConfigGetter — the only interface webhookHandler needs.
type mockConfigGetter struct {
	cfg    store.TriggerConfig
	getErr error
}

func (m *mockConfigGetter) GetTriggerConfig(_ context.Context, _ string) (store.TriggerConfig, error) {
	return m.cfg, m.getErr
}

// mockTriggerStore implements the full store.Store so fullMockStore (in manager_test.go)
// can embed it while satisfying NewManager's parameter type.
type mockTriggerStore struct {
	cfg    store.TriggerConfig
	getErr error
}

func (m *mockTriggerStore) GetTriggerConfig(_ context.Context, _ string) (store.TriggerConfig, error) {
	return m.cfg, m.getErr
}
func (m *mockTriggerStore) CreateWorkflow(_ context.Context, w store.Workflow) (store.Workflow, error) {
	return store.Workflow{}, nil
}
func (m *mockTriggerStore) GetWorkflow(_ context.Context, _ string) (store.Workflow, error) {
	return store.Workflow{}, nil
}
func (m *mockTriggerStore) ListWorkflows(_ context.Context) ([]store.WorkflowSummary, error) {
	return nil, nil
}
func (m *mockTriggerStore) UpdateWorkflow(_ context.Context, w store.Workflow) (store.Workflow, error) {
	return w, nil
}
func (m *mockTriggerStore) DeleteWorkflow(_ context.Context, _ string) error { return nil }
func (m *mockTriggerStore) GetWorkflowSchema(_ context.Context, _ string) (json.RawMessage, error) {
	return nil, nil
}
func (m *mockTriggerStore) CreateRun(_ context.Context, r store.Run) (store.Run, error) {
	return r, nil
}
func (m *mockTriggerStore) UpdateRunStatus(_ context.Context, _ string, _ store.RunStatus, _ map[string]any) error {
	return nil
}
func (m *mockTriggerStore) GetRun(_ context.Context, _ string) (store.Run, error) {
	return store.Run{}, nil
}
func (m *mockTriggerStore) ListRuns(_ context.Context, _ store.RunFilter) ([]store.Run, error) {
	return nil, nil
}
func (m *mockTriggerStore) UpsertChunks(_ context.Context, _ []store.RAGChunk) error { return nil }
func (m *mockTriggerStore) SearchChunks(_ context.Context, _ []float32, _ int, _ string) ([]store.RAGChunkResult, error) {
	return nil, nil
}
func (m *mockTriggerStore) SaveTriggerConfig(_ context.Context, _ string, _ store.TriggerConfig) error {
	return nil
}
func (m *mockTriggerStore) ListTriggerConfigs(_ context.Context) ([]store.WorkflowTrigger, error) {
	return nil, nil
}

type mockDispatcher struct {
	runID   string
	dispErr error
}

func (m *mockDispatcher) Dispatch(_ context.Context, _ RunRequest) (string, error) {
	return m.runID, m.dispErr
}

// ---- helpers ----------------------------------------------------------------

func newWebhookTestServer(t *testing.T, st triggerConfigGetter, disp Dispatcher) *httptest.Server {
	t.Helper()
	h := &webhookHandler{store: st, dispatcher: disp}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /webhooks/{workflow_id}", h.handle)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func postWebhook(t *testing.T, srv *httptest.Server, workflowID, body string) *http.Response {
	t.Helper()
	var bodyReader *strings.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	} else {
		bodyReader = strings.NewReader("")
	}
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/webhooks/"+workflowID, bodyReader)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /webhooks/%s: %v", workflowID, err)
	}
	return resp
}

// ---- tests ------------------------------------------------------------------

func TestWebhookHandler_DispatchesRun(t *testing.T) {
	ms := &mockConfigGetter{cfg: store.TriggerConfig{Kind: "webhook"}}
	disp := &mockDispatcher{runID: "run-xyz"}
	srv := newWebhookTestServer(t, ms, disp)

	resp := postWebhook(t, srv, "wf-1", `{"customer_id":42}`)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("want 202, got %d", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["run_id"] != "run-xyz" {
		t.Errorf("want run_id=run-xyz, got %v", body["run_id"])
	}
}

func TestWebhookHandler_EmptyBody(t *testing.T) {
	ms := &mockConfigGetter{cfg: store.TriggerConfig{Kind: "webhook"}}
	disp := &mockDispatcher{runID: "run-empty"}
	srv := newWebhookTestServer(t, ms, disp)

	resp := postWebhook(t, srv, "wf-1", "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("want 202, got %d", resp.StatusCode)
	}
}

func TestWebhookHandler_WorkflowNotFound(t *testing.T) {
	ms := &mockConfigGetter{getErr: store.ErrNotFound}
	disp := &mockDispatcher{}
	srv := newWebhookTestServer(t, ms, disp)

	resp := postWebhook(t, srv, "ghost", "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404, got %d", resp.StatusCode)
	}
}

func TestWebhookHandler_StoreError(t *testing.T) {
	ms := &mockConfigGetter{getErr: errors.New("db down")}
	disp := &mockDispatcher{}
	srv := newWebhookTestServer(t, ms, disp)

	resp := postWebhook(t, srv, "wf-1", "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", resp.StatusCode)
	}
}

func TestWebhookHandler_NotWebhookTrigger(t *testing.T) {
	ms := &mockConfigGetter{cfg: store.TriggerConfig{Kind: "manual"}}
	disp := &mockDispatcher{}
	srv := newWebhookTestServer(t, ms, disp)

	resp := postWebhook(t, srv, "wf-1", "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	errObj, _ := body["error"].(map[string]any)
	if errObj["code"] != "INVALID_TRIGGER" {
		t.Errorf("want INVALID_TRIGGER, got %v", errObj["code"])
	}
}

func TestWebhookHandler_DispatchError(t *testing.T) {
	ms := &mockConfigGetter{cfg: store.TriggerConfig{Kind: "webhook"}}
	disp := &mockDispatcher{dispErr: errors.New("engine down")}
	srv := newWebhookTestServer(t, ms, disp)

	resp := postWebhook(t, srv, "wf-1", "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", resp.StatusCode)
	}
}
