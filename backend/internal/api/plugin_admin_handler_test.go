package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/g8rswimmer/cogniflow/internal/node"
	nodeplugin "github.com/g8rswimmer/cogniflow/internal/node/plugin"
	"github.com/g8rswimmer/cogniflow/internal/store"
)

// stubNodeHandler is a minimal NodeHandler used to pre-populate the registry.
type stubNodeHandler struct{ typeID string }

func (s *stubNodeHandler) Meta() node.NodeMeta { return node.NodeMeta{TypeID: s.typeID} }
func (s *stubNodeHandler) Execute(_ context.Context, _ node.NodeInput) (node.NodeOutput, error) {
	return node.NodeOutput{}, nil
}

func newPluginAdminHandler(t *testing.T) (*pluginAdminHandler, *mockStore, *node.NodeRegistry) {
	t.Helper()
	st := newMockStore()
	registry := node.NewRegistry()
	return &pluginAdminHandler{
		store:      st,
		registry:   registry,
		registerFn: nodeplugin.RegisterOne,
	}, st, registry
}

// ── GET /v1/admin/plugins ─────────────────────────────────────────────────

func TestPluginAdmin_List_Empty(t *testing.T) {
	h, _, _ := newPluginAdminHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/plugins", nil)
	w := httptest.NewRecorder()
	h.list(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	plugins, _ := body["plugins"].([]any) // comma-ok: null marshals to nil, not []any
	if len(plugins) != 0 {
		t.Errorf("want 0 plugins, got %d", len(plugins))
	}
}

func TestPluginAdmin_List_ReturnsStored(t *testing.T) {
	h, st, _ := newPluginAdminHandler(t)
	_ = st.SavePluginRegistration(context.Background(), store.PluginRegistration{
		TypeID:       "test.plugin",
		Address:      "localhost:50051",
		DisplayName:  "Test",
		Category:     "plugin",
		InputSchema:  json.RawMessage(`{}`),
		OutputSchema: json.RawMessage(`{}`),
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/admin/plugins", nil)
	w := httptest.NewRecorder()
	h.list(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	plugins, _ := body["plugins"].([]any)
	if len(plugins) != 1 {
		t.Errorf("want 1 plugin, got %d", len(plugins))
	}
}

func TestPluginAdmin_List_StoreError(t *testing.T) {
	h, st, _ := newPluginAdminHandler(t)
	st.listPluginsErr = errInternal

	req := httptest.NewRequest(http.MethodGet, "/v1/admin/plugins", nil)
	w := httptest.NewRecorder()
	h.list(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", w.Code)
	}
}

// ── POST /v1/admin/plugins ────────────────────────────────────────────────

func TestPluginAdmin_Register_MissingAddress(t *testing.T) {
	h, _, _ := newPluginAdminHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/plugins",
		strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	h.register(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
	assertErrorCode(t, w.Body.Bytes(), "VALIDATION_FAILED")
}

func TestPluginAdmin_Register_InvalidJSON(t *testing.T) {
	h, _, _ := newPluginAdminHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/plugins",
		strings.NewReader(`not-json`))
	w := httptest.NewRecorder()
	h.register(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestPluginAdmin_Register_UnreachableAddress(t *testing.T) {
	h, _, _ := newPluginAdminHandler(t)
	// Port 1 is never open; the Meta() RPC will fail.
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/plugins",
		strings.NewReader(`{"address":"127.0.0.1:1"}`))
	w := httptest.NewRecorder()
	h.register(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("want 502, got %d", w.Code)
	}
	assertErrorCode(t, w.Body.Bytes(), "PLUGIN_UNAVAILABLE")
}

func TestPluginAdmin_Register_DuplicateTypeID(t *testing.T) {
	h, _, registry := newPluginAdminHandler(t)
	registry.Register(&stubNodeHandler{typeID: "echo.passthrough"})

	// Inject a stub registerFn that returns ErrDuplicateTypeID, simulating what
	// dialAndRegister returns when TryRegister finds the type already loaded.
	h.registerFn = func(_ context.Context, _ string, _ *node.NodeRegistry) (store.PluginRegistration, error) {
		return store.PluginRegistration{}, fmt.Errorf("%w %q", node.ErrDuplicateTypeID, "echo.passthrough")
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/admin/plugins",
		strings.NewReader(`{"address":"localhost:50051"}`))
	w := httptest.NewRecorder()
	h.register(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d: %s", w.Code, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "PLUGIN_ALREADY_REGISTERED")
}

// ── DELETE /v1/admin/plugins/{type_id} ───────────────────────────────────

func TestPluginAdmin_Deregister_NotFound(t *testing.T) {
	h, _, _ := newPluginAdminHandler(t)
	req := httptest.NewRequest(http.MethodDelete, "/v1/admin/plugins/nonexistent", nil)
	req.SetPathValue("type_id", "nonexistent")
	w := httptest.NewRecorder()
	h.deregister(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
	assertErrorCode(t, w.Body.Bytes(), "NOT_FOUND")
}

func TestPluginAdmin_Deregister_Success(t *testing.T) {
	h, st, registry := newPluginAdminHandler(t)

	// Pre-register both in registry and store.
	registry.Register(&stubNodeHandler{typeID: "test.plugin"})
	_ = st.SavePluginRegistration(context.Background(), store.PluginRegistration{
		TypeID:       "test.plugin",
		InputSchema:  json.RawMessage(`{}`),
		OutputSchema: json.RawMessage(`{}`),
	})

	req := httptest.NewRequest(http.MethodDelete, "/v1/admin/plugins/test.plugin", nil)
	req.SetPathValue("type_id", "test.plugin")
	w := httptest.NewRecorder()
	h.deregister(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", w.Code, w.Body.String())
	}

	// Confirm removed from registry.
	if _, err := registry.Lookup("test.plugin"); err == nil {
		t.Error("expected test.plugin to be removed from registry")
	}

	// Confirm removed from store.
	if _, err := st.GetPluginRegistration(context.Background(), "test.plugin"); err == nil {
		t.Error("expected test.plugin to be removed from store")
	}
}

// TestPluginAdmin_Deregister_BuiltInGuard verifies that a built-in node (one
// that is in the registry but has no persisted admin record) cannot be deleted.
func TestPluginAdmin_Deregister_BuiltInGuard(t *testing.T) {
	h, _, registry := newPluginAdminHandler(t)
	// Register a handler directly (simulates a built-in, never persisted via admin API).
	registry.Register(&stubNodeHandler{typeID: "builtin.node"})

	req := httptest.NewRequest(http.MethodDelete, "/v1/admin/plugins/builtin.node", nil)
	req.SetPathValue("type_id", "builtin.node")
	w := httptest.NewRecorder()
	h.deregister(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d: %s", w.Code, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "NOT_FOUND")

	// Built-in must still be in the registry.
	if _, err := registry.Lookup("builtin.node"); err != nil {
		t.Error("built-in node was incorrectly removed from registry")
	}
}

// ── PUT /v1/admin/plugins/{type_id} ──────────────────────────────────────

func TestPluginAdmin_Update_NotInStore(t *testing.T) {
	h, _, _ := newPluginAdminHandler(t)
	req := httptest.NewRequest(http.MethodPut, "/v1/admin/plugins/unknown",
		strings.NewReader(`{"address":"localhost:50051"}`))
	req.SetPathValue("type_id", "unknown")
	w := httptest.NewRecorder()
	h.update(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
	assertErrorCode(t, w.Body.Bytes(), "NOT_FOUND")
}

func TestPluginAdmin_Update_MissingAddress(t *testing.T) {
	h, st, _ := newPluginAdminHandler(t)
	_ = st.SavePluginRegistration(context.Background(), store.PluginRegistration{
		TypeID:       "test.plugin",
		InputSchema:  json.RawMessage(`{}`),
		OutputSchema: json.RawMessage(`{}`),
	})

	req := httptest.NewRequest(http.MethodPut, "/v1/admin/plugins/test.plugin",
		strings.NewReader(`{}`))
	req.SetPathValue("type_id", "test.plugin")
	w := httptest.NewRecorder()
	h.update(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestPluginAdmin_Update_UnreachableAddress(t *testing.T) {
	h, st, _ := newPluginAdminHandler(t)
	_ = st.SavePluginRegistration(context.Background(), store.PluginRegistration{
		TypeID:       "test.plugin",
		InputSchema:  json.RawMessage(`{}`),
		OutputSchema: json.RawMessage(`{}`),
	})

	req := httptest.NewRequest(http.MethodPut, "/v1/admin/plugins/test.plugin",
		strings.NewReader(`{"address":"127.0.0.1:1"}`))
	req.SetPathValue("type_id", "test.plugin")
	w := httptest.NewRecorder()
	h.update(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("want 502, got %d", w.Code)
	}
	assertErrorCode(t, w.Body.Bytes(), "PLUGIN_UNAVAILABLE")
}
