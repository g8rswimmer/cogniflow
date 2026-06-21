package grader_plugin

import (
	"errors"
	"testing"

	graderv1 "github.com/g8rswimmer/cogniflow/proto/grader/v1"
)

func makeProxy(typeID string) *grpcProxy {
	return &grpcProxy{
		meta:   GraderMeta{TypeID: typeID, DisplayName: typeID + " Plugin"},
		client: graderv1.NewGraderPluginClient(nil), // nil conn — not used in registry tests
	}
}

func TestGraderRegistry_Register_And_Get(t *testing.T) {
	r := NewGraderRegistry()
	p := makeProxy("test.grader")

	if err := r.Register(p); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, ok := r.Get("test.grader")
	if !ok {
		t.Fatal("Get: expected to find registered grader")
	}
	if got.meta.TypeID != "test.grader" {
		t.Errorf("want type_id test.grader, got %s", got.meta.TypeID)
	}
}

func TestGraderRegistry_Register_Duplicate(t *testing.T) {
	r := NewGraderRegistry()
	p := makeProxy("test.grader")
	_ = r.Register(p)

	err := r.Register(makeProxy("test.grader"))
	if err == nil {
		t.Fatal("expected error on duplicate registration")
	}
	if !errors.Is(err, ErrGraderAlreadyRegistered) {
		t.Errorf("expected ErrGraderAlreadyRegistered, got %v", err)
	}
}

func TestGraderRegistry_Get_NotFound(t *testing.T) {
	r := NewGraderRegistry()
	_, ok := r.Get("nonexistent")
	if ok {
		t.Error("expected Get to return false for unregistered type")
	}
}

func TestGraderRegistry_Replace(t *testing.T) {
	r := NewGraderRegistry()
	_ = r.Register(makeProxy("test.grader"))

	p2 := makeProxy("test.grader")
	r.Replace(p2)

	got, ok := r.Get("test.grader")
	if !ok {
		t.Fatal("expected grader after Replace")
	}
	if got != p2 {
		t.Error("expected registry to hold the replaced proxy")
	}
}

func TestGraderRegistry_Replace_NewEntry(t *testing.T) {
	r := NewGraderRegistry()
	p := makeProxy("new.grader")
	r.Replace(p)

	_, ok := r.Get("new.grader")
	if !ok {
		t.Error("Replace on new type_id should insert")
	}
}

func TestGraderRegistry_Unregister(t *testing.T) {
	r := NewGraderRegistry()
	_ = r.Register(makeProxy("test.grader"))

	if err := r.Unregister("test.grader"); err != nil {
		t.Fatalf("Unregister: %v", err)
	}

	_, ok := r.Get("test.grader")
	if ok {
		t.Error("expected grader to be removed after Unregister")
	}
}

func TestGraderRegistry_Unregister_NotFound(t *testing.T) {
	r := NewGraderRegistry()
	err := r.Unregister("nonexistent")
	if err == nil {
		t.Fatal("expected error for unregistered type_id")
	}
	if !errors.Is(err, ErrGraderNotFound) {
		t.Errorf("expected ErrGraderNotFound, got %v", err)
	}
}

func TestGraderRegistry_List(t *testing.T) {
	r := NewGraderRegistry()
	_ = r.Register(makeProxy("a.grader"))
	_ = r.Register(makeProxy("b.grader"))

	metas := r.List()
	if len(metas) != 2 {
		t.Errorf("want 2 entries, got %d", len(metas))
	}
}

func TestGraderRegistry_List_Empty(t *testing.T) {
	r := NewGraderRegistry()
	metas := r.List()
	if len(metas) != 0 {
		t.Errorf("want empty list, got %d entries", len(metas))
	}
}

func TestGraderRegistry_Shutdown(t *testing.T) {
	r := NewGraderRegistry()
	_ = r.Register(makeProxy("a.grader"))
	_ = r.Register(makeProxy("b.grader"))
	// Should not panic; nil conns Close gracefully.
	r.Shutdown()
}
