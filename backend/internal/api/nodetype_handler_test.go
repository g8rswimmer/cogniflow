package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/g8rswimmer/cogniflow/internal/node"
)

// testNode is a stub NodeHandler for nodetype handler tests.
type testNode struct {
	meta node.NodeMeta
}

func (n *testNode) Meta() node.NodeMeta { return n.meta }
func (n *testNode) Execute(_ context.Context, _ node.NodeInput) (node.NodeOutput, error) {
	return node.NodeOutput{}, nil
}

func makeTestNode(typeID, category string) *testNode {
	return &testNode{meta: node.NodeMeta{
		TypeID:       typeID,
		DisplayName:  typeID,
		Category:     category,
		Description:  "test",
		InputSchema:  json.RawMessage(`{}`),
		OutputSchema: json.RawMessage(`{}`),
	}}
}

func TestNodeTypeHandler_List_Empty(t *testing.T) {
	registry := node.NewRegistry()
	h := &nodeTypeHandler{registry: registry}

	r := httptest.NewRequest(http.MethodGet, "/node-types", nil)
	w := httptest.NewRecorder()
	h.list(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	types, ok := resp["node_types"].([]any)
	if !ok {
		t.Fatalf("expected node_types array, got %T", resp["node_types"])
	}
	if len(types) != 0 {
		t.Fatalf("expected empty list, got %d items", len(types))
	}
}

func TestNodeTypeHandler_List_ReturnsSortedTypes(t *testing.T) {
	registry := node.NewRegistry()
	registry.Register(makeTestNode("zzz.node", "ai"))
	registry.Register(makeTestNode("aaa.node", "deterministic"))
	registry.Register(makeTestNode("mmm.node", "plugin"))

	h := &nodeTypeHandler{registry: registry}
	r := httptest.NewRequest(http.MethodGet, "/node-types", nil)
	w := httptest.NewRecorder()
	h.list(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	types := resp["node_types"].([]any)
	if len(types) != 3 {
		t.Fatalf("expected 3 types, got %d", len(types))
	}

	first := types[0].(map[string]any)["type_id"].(string)
	if first != "aaa.node" {
		t.Fatalf("expected first type aaa.node, got %s", first)
	}
}

func TestNodeTypeHandler_List_HTTPRequestRegistered(t *testing.T) {
	registry := node.NewRegistry()
	registry.Register(makeTestNode("http.request", "deterministic"))

	h := &nodeTypeHandler{registry: registry}
	r := httptest.NewRequest(http.MethodGet, "/node-types", nil)
	w := httptest.NewRecorder()
	h.list(w, r)

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	types := resp["node_types"].([]any)
	found := false
	for _, t := range types {
		m := t.(map[string]any)
		if m["type_id"] == "http.request" {
			found = true
			if m["category"] != "deterministic" {
				break
			}
		}
	}
	if !found {
		t.Fatal("http.request node type not found in response")
	}
}

func TestNodeTypeHandler_List_ContentType(t *testing.T) {
	registry := node.NewRegistry()
	h := &nodeTypeHandler{registry: registry}

	r := httptest.NewRequest(http.MethodGet, "/node-types", nil)
	w := httptest.NewRecorder()
	h.list(w, r)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Fatalf("expected application/json, got %s", ct)
	}
}

func TestNodeTypeHandler_List_SchemaFields(t *testing.T) {
	registry := node.NewRegistry()
	registry.Register(makeTestNode("http.request", "deterministic"))

	h := &nodeTypeHandler{registry: registry}
	r := httptest.NewRequest(http.MethodGet, "/node-types", nil)
	w := httptest.NewRecorder()
	h.list(w, r)

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	types := resp["node_types"].([]any)
	nt := types[0].(map[string]any)

	fields := []string{"type_id", "display_name", "category", "description", "input_schema", "output_schema"}
	for _, f := range fields {
		if _, ok := nt[f]; !ok {
			t.Errorf("missing field %q in node type response", f)
		}
	}
}
