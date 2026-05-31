package node_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/g8rswimmer/cogniflow/internal/node"
)

// stubHandler is a minimal NodeHandler for testing.
type stubHandler struct {
	meta node.NodeMeta
}

func (s *stubHandler) Meta() node.NodeMeta { return s.meta }
func (s *stubHandler) Execute(_ context.Context, _ node.NodeInput) (node.NodeOutput, error) {
	return node.NodeOutput{}, nil
}

func newStub(typeID, category string) *stubHandler {
	return &stubHandler{meta: node.NodeMeta{
		TypeID:       typeID,
		DisplayName:  typeID + " display",
		Category:     category,
		Description:  "stub",
		InputSchema:  json.RawMessage(`{}`),
		OutputSchema: json.RawMessage(`{}`),
	}}
}

func TestRegistry_RegisterAndLookup(t *testing.T) {
	r := node.NewRegistry()
	h := newStub("test.alpha", "deterministic")
	r.Register(h)

	got, err := r.Lookup("test.alpha")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got.Meta().TypeID != "test.alpha" {
		t.Fatalf("expected type_id test.alpha, got %s", got.Meta().TypeID)
	}
}

func TestRegistry_LookupNotFound(t *testing.T) {
	r := node.NewRegistry()
	_, err := r.Lookup("does.not.exist")
	if err == nil {
		t.Fatal("expected error for unknown type_id")
	}
}

func TestRegistry_DuplicatePanics(t *testing.T) {
	r := node.NewRegistry()
	r.Register(newStub("test.dup", "ai"))

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on duplicate registration")
		}
	}()
	r.Register(newStub("test.dup", "ai"))
}

func TestRegistry_ListAllSorted(t *testing.T) {
	r := node.NewRegistry()
	r.Register(newStub("zzz.node", "ai"))
	r.Register(newStub("aaa.node", "deterministic"))
	r.Register(newStub("mmm.node", "plugin"))

	metas := r.ListAll()
	if len(metas) != 3 {
		t.Fatalf("expected 3 metas, got %d", len(metas))
	}
	if metas[0].TypeID != "aaa.node" || metas[1].TypeID != "mmm.node" || metas[2].TypeID != "zzz.node" {
		t.Fatalf("unexpected order: %v", metas)
	}
}

func TestRegistry_ListAllEmpty(t *testing.T) {
	r := node.NewRegistry()
	metas := r.ListAll()
	if len(metas) != 0 {
		t.Fatalf("expected empty list, got %d items", len(metas))
	}
}

func TestRegistry_ListAllIsSnapshot(t *testing.T) {
	r := node.NewRegistry()
	r.Register(newStub("snap.one", "deterministic"))
	metas1 := r.ListAll()

	r.Register(newStub("snap.two", "deterministic"))
	metas2 := r.ListAll()

	if len(metas1) != 1 {
		t.Fatalf("first snapshot should have 1 item, got %d", len(metas1))
	}
	if len(metas2) != 2 {
		t.Fatalf("second snapshot should have 2 items, got %d", len(metas2))
	}
}

func TestRegistry_ConcurrentRegisterLookup(t *testing.T) {
	r := node.NewRegistry()
	done := make(chan struct{})

	go func() {
		r.Register(newStub("concurrent.node", "ai"))
		close(done)
	}()
	<-done

	_, err := r.Lookup("concurrent.node")
	if err != nil {
		t.Fatalf("concurrent lookup failed: %v", err)
	}
}
