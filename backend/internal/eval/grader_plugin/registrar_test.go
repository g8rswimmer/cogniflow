package grader_plugin

import (
	"context"
	"errors"
	"testing"

	"github.com/g8rswimmer/cogniflow/internal/store"
	graderv1 "github.com/g8rswimmer/cogniflow/proto/grader/v1"
)

func TestRegisterOne_Success(t *testing.T) {
	conn := startTestGraderServer(t, &echoGraderServer{
		metaResp: &graderv1.MetaResponse{
			TypeId:       "my.grader",
			DisplayName:  "My Grader",
			Description:  "test",
			ConfigSchema: []byte(`{"type":"object"}`),
		},
	})
	addr := conn.Target()

	registry := NewGraderRegistry()
	reg, err := RegisterOne(context.Background(), addr, registry)
	if err != nil {
		t.Fatalf("RegisterOne: %v", err)
	}
	if reg.TypeID != "my.grader" {
		t.Errorf("want type_id my.grader, got %s", reg.TypeID)
	}
	if reg.Address != addr {
		t.Errorf("want address %s, got %s", addr, reg.Address)
	}

	_, ok := registry.Get("my.grader")
	if !ok {
		t.Error("expected grader to be in registry after RegisterOne")
	}
}

func TestRegisterOne_Duplicate(t *testing.T) {
	conn := startTestGraderServer(t, &echoGraderServer{
		metaResp: &graderv1.MetaResponse{TypeId: "dup.grader"},
	})
	addr := conn.Target()

	registry := NewGraderRegistry()
	_, err := RegisterOne(context.Background(), addr, registry)
	if err != nil {
		t.Fatalf("first RegisterOne: %v", err)
	}

	_, err = RegisterOne(context.Background(), addr, registry)
	if err == nil {
		t.Fatal("expected error on duplicate RegisterOne")
	}
	if !errors.Is(err, ErrGraderAlreadyRegistered) {
		t.Errorf("expected ErrGraderAlreadyRegistered, got %v", err)
	}
}

func TestRegisterOne_EmptyTypeID(t *testing.T) {
	conn := startTestGraderServer(t, &echoGraderServer{
		metaResp: &graderv1.MetaResponse{TypeId: ""},
	})
	addr := conn.Target()

	registry := NewGraderRegistry()
	_, err := RegisterOne(context.Background(), addr, registry)
	if err == nil {
		t.Fatal("expected error for empty type_id")
	}
}

func TestRegisterOne_InvalidConfigSchema(t *testing.T) {
	conn := startTestGraderServer(t, &echoGraderServer{
		metaResp: &graderv1.MetaResponse{
			TypeId:       "bad.schema",
			ConfigSchema: []byte(`not-json`),
		},
	})
	addr := conn.Target()

	registry := NewGraderRegistry()
	_, err := RegisterOne(context.Background(), addr, registry)
	if err == nil {
		t.Fatal("expected error for invalid config_schema JSON")
	}
}

func TestUpdateOne_Success(t *testing.T) {
	conn := startTestGraderServer(t, &echoGraderServer{
		metaResp: &graderv1.MetaResponse{TypeId: "upd.grader", DisplayName: "Updated"},
	})
	addr := conn.Target()

	registry := NewGraderRegistry()
	_, err := RegisterOne(context.Background(), addr, registry)
	if err != nil {
		t.Fatalf("RegisterOne: %v", err)
	}

	reg, err := UpdateOne(context.Background(), "upd.grader", addr, registry)
	if err != nil {
		t.Fatalf("UpdateOne: %v", err)
	}
	if reg.TypeID != "upd.grader" {
		t.Errorf("want type_id upd.grader, got %s", reg.TypeID)
	}
}

func TestUpdateOne_TypeIDMismatch(t *testing.T) {
	conn := startTestGraderServer(t, &echoGraderServer{
		metaResp: &graderv1.MetaResponse{TypeId: "actual.grader"},
	})
	addr := conn.Target()

	registry := NewGraderRegistry()
	_, err := UpdateOne(context.Background(), "expected.grader", addr, registry)
	if err == nil {
		t.Fatal("expected error on type_id mismatch")
	}
	if !errors.Is(err, ErrTypeIDMismatch) {
		t.Errorf("expected ErrTypeIDMismatch, got %v", err)
	}
}

func TestLoadFromStore_Success(t *testing.T) {
	conn := startTestGraderServer(t, &echoGraderServer{
		metaResp: &graderv1.MetaResponse{TypeId: "stored.grader"},
	})

	regs := []store.GraderRegistration{{TypeID: "stored.grader", Address: conn.Target()}}
	st := &fakeGraderStore{regs: regs}
	registry := NewGraderRegistry()

	LoadFromStore(context.Background(), st, registry)

	_, ok := registry.Get("stored.grader")
	if !ok {
		t.Error("expected stored.grader to be loaded from store")
	}
}

func TestLoadFromStore_StoreError(t *testing.T) {
	st := &fakeGraderStore{err: errors.New("db error")}
	registry := NewGraderRegistry()
	// Should not panic; errors are logged and skipped.
	LoadFromStore(context.Background(), st, registry)
}

func TestLoadFromStore_UnreachablePlugin(t *testing.T) {
	regs := []store.GraderRegistration{{TypeID: "unreachable", Address: "127.0.0.1:1"}}
	st := &fakeGraderStore{regs: regs}
	registry := NewGraderRegistry()
	// Should not panic; unreachable plugins are skipped.
	LoadFromStore(context.Background(), st, registry)
}

func TestNewPluginGrader_NotRegistered(t *testing.T) {
	registry := NewGraderRegistry()
	_, err := NewPluginGrader(registry, "nonexistent.grader", map[string]any{})
	if err == nil {
		t.Fatal("expected error for unregistered grader type")
	}
}

func TestNewPluginGrader_Success(t *testing.T) {
	conn := startTestGraderServer(t, &echoGraderServer{})
	proxy := &grpcProxy{
		meta:   GraderMeta{TypeID: "echo.grader"},
		client: graderv1.NewGraderPluginClient(conn),
	}
	registry := NewGraderRegistry()
	_ = registry.Register(proxy)

	g, err := NewPluginGrader(registry, "echo.grader", map[string]any{"key": "val"})
	if err != nil {
		t.Fatalf("NewPluginGrader: %v", err)
	}
	if g == nil {
		t.Fatal("expected non-nil PluginGrader")
	}
}

// fakeGraderStore implements graderPluginStoreIface for registrar tests.
type fakeGraderStore struct {
	regs []store.GraderRegistration
	err  error
}

func (f *fakeGraderStore) ListGraderRegistrations(_ context.Context) ([]store.GraderRegistration, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.regs, nil
}
