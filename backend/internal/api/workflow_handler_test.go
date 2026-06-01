package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	httprequest "github.com/g8rswimmer/cogniflow/internal/node/builtin/http_request"
	"github.com/g8rswimmer/cogniflow/internal/node"
	"github.com/g8rswimmer/cogniflow/internal/store"
)

// noopTriggerManager satisfies triggerManager with no-op behaviour for tests
// that focus on workflow CRUD and don't need to verify trigger side-effects.
type noopTriggerManager struct{}

func (noopTriggerManager) Upsert(_ string, _ store.TriggerConfig) error { return nil }
func (noopTriggerManager) Remove(_ string)                               {}

func setupWorkflowHandler(t *testing.T) (*workflowHandler, *mockStore) {
	t.Helper()
	ms := newMockStore()
	registry := node.NewRegistry()
	registry.Register(httprequest.New())
	return &workflowHandler{store: ms, registry: registry, triggers: noopTriggerManager{}}, ms
}

// ---- List ----------------------------------------------------------------

func TestWorkflowHandler_List_Empty(t *testing.T) {
	h, _ := setupWorkflowHandler(t)
	r := httptest.NewRequest("GET", "/workflows", nil)
	w := httptest.NewRecorder()
	h.list(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	workflows := resp["workflows"].([]any)
	if len(workflows) != 0 {
		t.Fatalf("expected empty list, got %v", workflows)
	}
}

func TestWorkflowHandler_List_WithWorkflows(t *testing.T) {
	h, ms := setupWorkflowHandler(t)
	ms.workflows["wf-1"] = store.Workflow{ID: "wf-1", Name: "First", Trigger: store.Trigger{Kind: "manual"}}
	ms.workflows["wf-2"] = store.Workflow{ID: "wf-2", Name: "Second", Trigger: store.Trigger{Kind: "cron"}}

	r := httptest.NewRequest("GET", "/workflows", nil)
	w := httptest.NewRecorder()
	h.list(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	workflows := resp["workflows"].([]any)
	if len(workflows) != 2 {
		t.Fatalf("expected 2 workflows, got %d", len(workflows))
	}
}

// ---- Create --------------------------------------------------------------

func TestWorkflowHandler_Create_Success(t *testing.T) {
	h, _ := setupWorkflowHandler(t)

	body := `{
		"name": "Test Flow",
		"trigger": {"kind": "manual"},
		"timeout_seconds": 60,
		"nodes": [
			{"id":"n1","type_id":"http.request","label":"Step 1","position":{"x":0,"y":0},"config":{"url":"https://example.com","method":"GET"}},
			{"id":"n2","type_id":"http.request","label":"Step 2","position":{"x":200,"y":0},"config":{"url":"https://example.com","method":"GET"}}
		],
		"edges": [{"id":"e1","source_id":"n1","target_id":"n2","branch_label":null}]
	}`
	r := httptest.NewRequest("POST", "/workflows", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.create(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["name"] != "Test Flow" {
		t.Fatalf("expected name='Test Flow', got %v", resp["name"])
	}
	if resp["id"] == nil || resp["id"] == "" {
		t.Fatal("expected id in response")
	}
}

func TestWorkflowHandler_Create_MissingName(t *testing.T) {
	h, _ := setupWorkflowHandler(t)

	body := `{"trigger":{"kind":"manual"}}`
	r := httptest.NewRequest("POST", "/workflows", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.create(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	assertErrorCode(t, w.Body.Bytes(), "VALIDATION_FAILED")
}

func TestWorkflowHandler_Create_InvalidJSON(t *testing.T) {
	h, _ := setupWorkflowHandler(t)

	r := httptest.NewRequest("POST", "/workflows", strings.NewReader("{bad json"))
	w := httptest.NewRecorder()
	h.create(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	assertErrorCode(t, w.Body.Bytes(), "VALIDATION_FAILED")
}

func TestWorkflowHandler_Create_CycleDetected(t *testing.T) {
	h, _ := setupWorkflowHandler(t)

	body := `{
		"name": "Cyclic",
		"trigger": {"kind": "manual"},
		"nodes": [
			{"id":"n1","type_id":"http.request","position":{"x":0,"y":0}},
			{"id":"n2","type_id":"http.request","position":{"x":0,"y":0}}
		],
		"edges": [
			{"id":"e1","source_id":"n1","target_id":"n2"},
			{"id":"e2","source_id":"n2","target_id":"n1"}
		]
	}`
	r := httptest.NewRequest("POST", "/workflows", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.create(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "CYCLE_DETECTED")
}

func TestWorkflowHandler_Create_WebhookURLInResponse(t *testing.T) {
	h, _ := setupWorkflowHandler(t)

	body := `{"name":"Hook","trigger":{"kind":"webhook"}}`
	r := httptest.NewRequest("POST", "/workflows", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.create(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	trigger := resp["trigger"].(map[string]any)
	webhookURL, _ := trigger["webhook_url"].(string)
	if webhookURL == "" {
		t.Fatalf("expected webhook_url in trigger, got %v", trigger)
	}
}

// ---- Get -----------------------------------------------------------------

func TestWorkflowHandler_Get_Found(t *testing.T) {
	h, ms := setupWorkflowHandler(t)
	ms.workflows["abc-123"] = store.Workflow{
		ID:      "abc-123",
		Name:    "My Flow",
		Trigger: store.Trigger{Kind: "manual"},
	}

	r := httptest.NewRequest("GET", "/workflows/abc-123", nil)
	r.SetPathValue("id", "abc-123")
	w := httptest.NewRecorder()
	h.get(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["name"] != "My Flow" {
		t.Fatalf("expected 'My Flow', got %v", resp["name"])
	}
}

func TestWorkflowHandler_Get_NotFound(t *testing.T) {
	h, _ := setupWorkflowHandler(t)

	r := httptest.NewRequest("GET", "/workflows/nope", nil)
	r.SetPathValue("id", "nope")
	w := httptest.NewRecorder()
	h.get(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
	assertErrorCode(t, w.Body.Bytes(), "NOT_FOUND")
}

func TestWorkflowHandler_Get_MasksSensitiveFields(t *testing.T) {
	h, ms := setupWorkflowHandler(t)
	ms.workflows["wf-sensitive"] = store.Workflow{
		ID:      "wf-sensitive",
		Name:    "Sensitive",
		Trigger: store.Trigger{Kind: "manual"},
		Nodes: []store.WorkflowNode{
			{
				ID:            "n1",
				TypeID:        "http.request",
				Config:        map[string]any{"url": "https://example.com", "api_key": "sk-secret"},
				SensitiveKeys: map[string]bool{"api_key": true, "url": false},
			},
		},
	}

	r := httptest.NewRequest("GET", "/workflows/wf-sensitive", nil)
	r.SetPathValue("id", "wf-sensitive")
	w := httptest.NewRecorder()
	h.get(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	nodes := resp["nodes"].([]any)
	config := nodes[0].(map[string]any)["config"].(map[string]any)
	if config["api_key"] != "***" {
		t.Fatalf("expected api_key='***', got %v", config["api_key"])
	}
	if config["url"] != "https://example.com" {
		t.Fatalf("expected url unmasked, got %v", config["url"])
	}
}

// ---- Update --------------------------------------------------------------

func TestWorkflowHandler_Update_Success(t *testing.T) {
	h, ms := setupWorkflowHandler(t)
	ms.workflows["upd-1"] = store.Workflow{
		ID:      "upd-1",
		Name:    "Original",
		Trigger: store.Trigger{Kind: "manual"},
	}

	body := `{"name":"Updated","trigger":{"kind":"manual"}}`
	r := httptest.NewRequest("PUT", "/workflows/upd-1", strings.NewReader(body))
	r.SetPathValue("id", "upd-1")
	w := httptest.NewRecorder()
	h.update(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["name"] != "Updated" {
		t.Fatalf("expected name='Updated', got %v", resp["name"])
	}
}

func TestWorkflowHandler_Update_NotFound(t *testing.T) {
	h, _ := setupWorkflowHandler(t)

	body := `{"name":"Ghost","trigger":{"kind":"manual"}}`
	r := httptest.NewRequest("PUT", "/workflows/ghost", strings.NewReader(body))
	r.SetPathValue("id", "ghost")
	w := httptest.NewRecorder()
	h.update(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
	assertErrorCode(t, w.Body.Bytes(), "NOT_FOUND")
}

func TestWorkflowHandler_Update_CycleDetected(t *testing.T) {
	h, ms := setupWorkflowHandler(t)
	ms.workflows["cyc-1"] = store.Workflow{ID: "cyc-1", Name: "Cyclic", Trigger: store.Trigger{Kind: "manual"}}

	body := `{
		"name":"Cyclic",
		"trigger":{"kind":"manual"},
		"nodes":[
			{"id":"n1","type_id":"http.request","position":{"x":0,"y":0}},
			{"id":"n2","type_id":"http.request","position":{"x":0,"y":0}}
		],
		"edges":[
			{"id":"e1","source_id":"n1","target_id":"n2"},
			{"id":"e2","source_id":"n2","target_id":"n1"}
		]
	}`
	r := httptest.NewRequest("PUT", "/workflows/cyc-1", strings.NewReader(body))
	r.SetPathValue("id", "cyc-1")
	w := httptest.NewRecorder()
	h.update(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	assertErrorCode(t, w.Body.Bytes(), "CYCLE_DETECTED")
}

// ---- Delete --------------------------------------------------------------

func TestWorkflowHandler_Delete_Success(t *testing.T) {
	h, ms := setupWorkflowHandler(t)
	ms.workflows["del-1"] = store.Workflow{ID: "del-1", Name: "To Delete", Trigger: store.Trigger{Kind: "manual"}}

	r := httptest.NewRequest("DELETE", "/workflows/del-1", nil)
	r.SetPathValue("id", "del-1")
	w := httptest.NewRecorder()
	h.delete(w, r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
	if _, ok := ms.workflows["del-1"]; ok {
		t.Fatal("workflow should be deleted")
	}
}

func TestWorkflowHandler_Delete_NotFound(t *testing.T) {
	h, _ := setupWorkflowHandler(t)

	r := httptest.NewRequest("DELETE", "/workflows/missing", nil)
	r.SetPathValue("id", "missing")
	w := httptest.NewRecorder()
	h.delete(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
	assertErrorCode(t, w.Body.Bytes(), "NOT_FOUND")
}

// ---- Error path tests ---------------------------------------------------

func TestWorkflowHandler_List_StoreError(t *testing.T) {
	h, ms := setupWorkflowHandler(t)
	ms.listErr = errInternal

	r := httptest.NewRequest("GET", "/workflows", nil)
	w := httptest.NewRecorder()
	h.list(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
	assertErrorCode(t, w.Body.Bytes(), "INTERNAL_ERROR")
}

func TestWorkflowHandler_Create_StoreError(t *testing.T) {
	h, ms := setupWorkflowHandler(t)
	ms.createErr = errInternal

	body := `{"name":"Fail","trigger":{"kind":"manual"}}`
	r := httptest.NewRequest("POST", "/workflows", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.create(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d body=%s", w.Code, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "WORKFLOW_SAVE_FAILED")
}

func TestWorkflowHandler_Get_InternalError(t *testing.T) {
	h, ms := setupWorkflowHandler(t)
	ms.getErr = errInternal

	r := httptest.NewRequest("GET", "/workflows/any", nil)
	r.SetPathValue("id", "any")
	w := httptest.NewRecorder()
	h.get(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
	assertErrorCode(t, w.Body.Bytes(), "INTERNAL_ERROR")
}

func TestWorkflowHandler_Update_StoreError(t *testing.T) {
	h, ms := setupWorkflowHandler(t)
	ms.workflows["err-wf"] = store.Workflow{ID: "err-wf", Name: "Original", Trigger: store.Trigger{Kind: "manual"}}
	ms.updateErr = errInternal

	body := `{"name":"Updated","trigger":{"kind":"manual"}}`
	r := httptest.NewRequest("PUT", "/workflows/err-wf", strings.NewReader(body))
	r.SetPathValue("id", "err-wf")
	w := httptest.NewRecorder()
	h.update(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d body=%s", w.Code, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "WORKFLOW_SAVE_FAILED")
}

func TestWorkflowHandler_Delete_StoreError(t *testing.T) {
	h, ms := setupWorkflowHandler(t)
	ms.workflows["err-del"] = store.Workflow{ID: "err-del", Name: "X", Trigger: store.Trigger{Kind: "manual"}}
	ms.deleteErr = errInternal

	r := httptest.NewRequest("DELETE", "/workflows/err-del", nil)
	r.SetPathValue("id", "err-del")
	w := httptest.NewRecorder()
	h.delete(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
	assertErrorCode(t, w.Body.Bytes(), "INTERNAL_ERROR")
}

func TestWorkflowHandler_Update_InvalidJSON(t *testing.T) {
	h, _ := setupWorkflowHandler(t)

	r := httptest.NewRequest("PUT", "/workflows/any", strings.NewReader("{bad"))
	r.SetPathValue("id", "any")
	w := httptest.NewRecorder()
	h.update(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	assertErrorCode(t, w.Body.Bytes(), "VALIDATION_FAILED")
}

func TestWorkflowHandler_Update_MissingName(t *testing.T) {
	h, _ := setupWorkflowHandler(t)

	r := httptest.NewRequest("PUT", "/workflows/any", strings.NewReader(`{"trigger":{"kind":"manual"}}`))
	r.SetPathValue("id", "any")
	w := httptest.NewRecorder()
	h.update(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	assertErrorCode(t, w.Body.Bytes(), "VALIDATION_FAILED")
}

func TestWorkflowHandler_Create_InvalidTemplate(t *testing.T) {
	h, _ := setupWorkflowHandler(t)

	body := `{
		"name": "Bad Template",
		"trigger": {"kind": "manual"},
		"nodes": [{"id":"n1","type_id":"http.request","position":{"x":0,"y":0},
		           "config":{"url":"{{.broken","method":"GET"}}],
		"edges": []
	}`
	r := httptest.NewRequest("POST", "/workflows", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.create(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "VALIDATION_FAILED")
}

func TestWorkflowHandler_Create_ValidTemplate(t *testing.T) {
	h, _ := setupWorkflowHandler(t)

	body := `{
		"name": "Template Flow",
		"trigger": {"kind": "manual"},
		"nodes": [
			{"id":"n1","type_id":"http.request","position":{"x":0,"y":0},
			 "config":{"url":"https://example.com","method":"GET"}},
			{"id":"n2","type_id":"http.request","position":{"x":200,"y":0},
			 "config":{"url":"https://example.com/{{.n1.body}}","method":"GET"}}
		],
		"edges": [{"id":"e1","source_id":"n1","target_id":"n2"}]
	}`
	r := httptest.NewRequest("POST", "/workflows", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.create(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestWorkflowHandler_Update_InvalidTemplate(t *testing.T) {
	h, ms := setupWorkflowHandler(t)
	ms.workflows["wf-1"] = store.Workflow{ID: "wf-1", Name: "Flow", Trigger: store.Trigger{Kind: "manual"}}

	body := `{
		"name": "Flow",
		"trigger": {"kind": "manual"},
		"nodes": [{"id":"n1","type_id":"http.request","position":{"x":0,"y":0},
		           "config":{"url":"{{unclosed","method":"GET"}}],
		"edges": []
	}`
	r := httptest.NewRequest("PUT", "/workflows/wf-1", strings.NewReader(body))
	r.SetPathValue("id", "wf-1")
	w := httptest.NewRecorder()
	h.update(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "VALIDATION_FAILED")
}

func TestWorkflowHandler_Create_LiteralURLNotTemplate(t *testing.T) {
	h, _ := setupWorkflowHandler(t)

	// Plain URL with no {{ }} — should never be treated as a template
	body := `{
		"name": "Literal URL",
		"trigger": {"kind": "manual"},
		"nodes": [{"id":"n1","type_id":"http.request","position":{"x":0,"y":0},
		           "config":{"url":"https://example.com/items","method":"GET"}}],
		"edges": []
	}`
	r := httptest.NewRequest("POST", "/workflows", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.create(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestWorkflowHandler_Create_InvalidHeaderTemplate(t *testing.T) {
	h, _ := setupWorkflowHandler(t)

	// A malformed template inside the headers map (additionalProperties) must be caught.
	body := `{
		"name": "Bad Header Template",
		"trigger": {"kind": "manual"},
		"nodes": [{"id":"n1","type_id":"http.request","position":{"x":0,"y":0},
		           "config":{"url":"https://example.com","method":"GET",
		                     "headers":{"Authorization":"{{.bad"}}}],
		"edges": []
	}`
	r := httptest.NewRequest("POST", "/workflows", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.create(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "VALIDATION_FAILED")
}

func TestWorkflowHandler_Create_ValidHeaderTemplate(t *testing.T) {
	h, _ := setupWorkflowHandler(t)

	body := `{
		"name": "Valid Header Template",
		"trigger": {"kind": "manual"},
		"nodes": [{"id":"n1","type_id":"http.request","position":{"x":0,"y":0},
		           "config":{"url":"https://example.com","method":"GET",
		                     "headers":{"Authorization":"Bearer {{.n0.token}}"}}}],
		"edges": []
	}`
	r := httptest.NewRequest("POST", "/workflows", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.create(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestWorkflowHandler_Create_UnknownNodeType(t *testing.T) {
	h, _ := setupWorkflowHandler(t)

	body := `{
		"name": "Unknown Node",
		"trigger": {"kind": "manual"},
		"nodes": [{"id":"n1","type_id":"does.not.exist","position":{"x":0,"y":0}}],
		"edges": []
	}`
	r := httptest.NewRequest("POST", "/workflows", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.create(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "VALIDATION_FAILED")
}

func TestWorkflowHandler_Create_EdgeReferencesBadNode(t *testing.T) {
	h, _ := setupWorkflowHandler(t)

	body := `{
		"name": "Bad Edge",
		"trigger": {"kind": "manual"},
		"nodes": [{"id":"n1","type_id":"http.request","position":{"x":0,"y":0}}],
		"edges": [{"id":"e1","source_id":"n1","target_id":"ghost"}]
	}`
	r := httptest.NewRequest("POST", "/workflows", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.create(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "VALIDATION_FAILED")
}

// ---- helpers -------------------------------------------------------------

func assertErrorCode(t *testing.T, body []byte, code string) {
	t.Helper()
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}
	errObj, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'error' object in response, got %v", resp)
	}
	if errObj["code"] != code {
		t.Fatalf("expected error code %q, got %v", code, errObj["code"])
	}
}
