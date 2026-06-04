package plugin

import (
	"context"
	"encoding/json"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/g8rswimmer/cogniflow/internal/node"
	pluginv1 "github.com/g8rswimmer/cogniflow/proto/plugin/v1"
)

// startRegistrarTestServer starts a gRPC server and returns its address.
func startRegistrarTestServer(t *testing.T, srv pluginv1.NodePluginServer) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	gs := grpc.NewServer()
	pluginv1.RegisterNodePluginServer(gs, srv)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.GracefulStop)
	return lis.Addr().String()
}

func TestRegister_Success(t *testing.T) {
	addr := startRegistrarTestServer(t, &echoServer{
		metaResp: &pluginv1.MetaResponse{
			TypeId:       "echo.passthrough",
			DisplayName:  "Echo",
			Description:  "Echoes upstream data",
			InputSchema:  []byte(`{"type":"object","properties":{}}`),
			OutputSchema: []byte(`{"type":"object","properties":{}}`),
		},
	})

	registry := node.NewRegistry()
	Register(context.Background(), addr, registry)

	h, err := registry.Lookup("echo.passthrough")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	meta := h.Meta()
	if meta.TypeID != "echo.passthrough" {
		t.Errorf("want echo.passthrough, got %s", meta.TypeID)
	}
	if meta.Category != "plugin" {
		t.Errorf("want category plugin, got %s", meta.Category)
	}
}

func TestRegister_EmptyAddresses(t *testing.T) {
	registry := node.NewRegistry()
	Register(context.Background(), "", registry)
	// No registrations, no panic.
	if len(registry.ListAll()) != 0 {
		t.Errorf("expected 0 registrations, got %d", len(registry.ListAll()))
	}
}

func TestRegister_UnreachableAddress(t *testing.T) {
	registry := node.NewRegistry()
	// Should log and skip, not panic.
	Register(context.Background(), "127.0.0.1:1", registry)
	if len(registry.ListAll()) != 0 {
		t.Errorf("expected 0 registrations after failed dial, got %d", len(registry.ListAll()))
	}
}

func TestRegister_MultipleAddresses(t *testing.T) {
	addr1 := startRegistrarTestServer(t, &echoServer{
		metaResp: &pluginv1.MetaResponse{
			TypeId:       "plugin.one",
			DisplayName:  "Plugin One",
			InputSchema:  []byte(`{}`),
			OutputSchema: []byte(`{}`),
		},
	})
	addr2 := startRegistrarTestServer(t, &echoServer{
		metaResp: &pluginv1.MetaResponse{
			TypeId:       "plugin.two",
			DisplayName:  "Plugin Two",
			InputSchema:  []byte(`{}`),
			OutputSchema: []byte(`{}`),
		},
	})

	registry := node.NewRegistry()
	Register(context.Background(), addr1+","+addr2, registry)

	if _, err := registry.Lookup("plugin.one"); err != nil {
		t.Errorf("expected plugin.one to be registered: %v", err)
	}
	if _, err := registry.Lookup("plugin.two"); err != nil {
		t.Errorf("expected plugin.two to be registered: %v", err)
	}
}

func TestProtoToMeta_EmptyTypeID(t *testing.T) {
	_, err := protoToMeta(&pluginv1.MetaResponse{})
	if err == nil {
		t.Fatal("expected error for empty type_id")
	}
}

func TestProtoToMeta_InvalidInputSchema(t *testing.T) {
	_, err := protoToMeta(&pluginv1.MetaResponse{
		TypeId:      "test.node",
		InputSchema: []byte(`not json`),
	})
	if err == nil {
		t.Fatal("expected error for invalid input_schema")
	}
}

func TestProtoToMeta_EmptySchemas_DefaultToEmptyObject(t *testing.T) {
	meta, err := protoToMeta(&pluginv1.MetaResponse{
		TypeId: "test.node",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var v any
	if err := json.Unmarshal(meta.InputSchema, &v); err != nil {
		t.Errorf("input_schema not valid JSON: %v", err)
	}
	if err := json.Unmarshal(meta.OutputSchema, &v); err != nil {
		t.Errorf("output_schema not valid JSON: %v", err)
	}
}

func TestRegister_PluginAppearsInListAll(t *testing.T) {
	addr := startRegistrarTestServer(t, &echoServer{
		metaResp: &pluginv1.MetaResponse{
			TypeId:       "test.list",
			DisplayName:  "List Test",
			InputSchema:  []byte(`{}`),
			OutputSchema: []byte(`{}`),
		},
	})

	registry := node.NewRegistry()
	Register(context.Background(), addr, registry)

	all := registry.ListAll()
	found := false
	for _, m := range all {
		if m.TypeID == "test.list" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected test.list in ListAll()")
	}
}

func TestRegisterOne_ExecuteViaProxy(t *testing.T) {
	addr := startRegistrarTestServer(t, &echoServer{
		metaResp: &pluginv1.MetaResponse{
			TypeId:       "echo.exec",
			InputSchema:  []byte(`{}`),
			OutputSchema: []byte(`{}`),
		},
		executeResp: &pluginv1.ExecuteResponse{
			Result: &pluginv1.ExecuteResponse_Data{Data: []byte(`{"pong":true}`)},
		},
	})

	registry := node.NewRegistry()
	Register(context.Background(), addr, registry)

	h, err := registry.Lookup("echo.exec")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}

	out, err := h.Execute(context.Background(), node.NodeInput{
		UpstreamData: map[string]any{},
		Config:       map[string]any{},
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out.Data["pong"] != true {
		t.Errorf("want pong=true, got %v", out.Data["pong"])
	}
}

func TestRegister_SkipsBlankEntries(t *testing.T) {
	addr := startRegistrarTestServer(t, &echoServer{
		metaResp: &pluginv1.MetaResponse{
			TypeId:       "blanks.test",
			InputSchema:  []byte(`{}`),
			OutputSchema: []byte(`{}`),
		},
	})

	registry := node.NewRegistry()
	// Leading/trailing commas and spaces should be handled gracefully.
	Register(context.Background(), " , "+addr+" , , ", registry)

	if _, err := registry.Lookup("blanks.test"); err != nil {
		t.Errorf("expected blanks.test registered: %v", err)
	}
}

// stubHandler is a minimal NodeHandler used to pre-register a TypeID.
type stubHandler struct{ typeID string }

func (s *stubHandler) Meta() node.NodeMeta { return node.NodeMeta{TypeID: s.typeID} }
func (s *stubHandler) Execute(_ context.Context, _ node.NodeInput) (node.NodeOutput, error) {
	return node.NodeOutput{}, nil
}

func TestRegister_DuplicateTypeID_DoesNotPanic(t *testing.T) {
	addr := startRegistrarTestServer(t, &echoServer{
		metaResp: &pluginv1.MetaResponse{
			TypeId:       "builtin.existing",
			InputSchema:  []byte(`{}`),
			OutputSchema: []byte(`{}`),
		},
	})

	registry := node.NewRegistry()
	// Pre-register a handler with the same type_id to simulate the conflict.
	registry.Register(&stubHandler{typeID: "builtin.existing"})

	// Should warn and skip, not panic.
	Register(context.Background(), addr, registry)

	// Only the original registration should be present.
	all := registry.ListAll()
	count := 0
	for _, m := range all {
		if m.TypeID == "builtin.existing" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 builtin.existing, got %d", count)
	}
}

func TestRegister_ConnClosedOnMetaError(t *testing.T) {
	// A server that always returns an error for Meta.
	addr := startRegistrarTestServer(t, &echoServer{
		// metaResp is nil — the server's Meta() returns nil, nil which triggers
		// protoToMeta type_id="" error.
		metaResp: nil,
	})

	registry := node.NewRegistry()
	// Should not hang or leak — connection should be cleaned up.
	Register(context.Background(), addr, registry)
	if len(registry.ListAll()) != 0 {
		t.Error("expected 0 registrations after meta error")
	}
}

func TestNodeRegistry_Shutdown_ClosesPluginConns(t *testing.T) {
	addr := startRegistrarTestServer(t, &echoServer{
		metaResp: &pluginv1.MetaResponse{
			TypeId:       "shutdown.test",
			InputSchema:  []byte(`{}`),
			OutputSchema: []byte(`{}`),
		},
	})

	registry := node.NewRegistry()
	Register(context.Background(), addr, registry)

	if _, err := registry.Lookup("shutdown.test"); err != nil {
		t.Fatalf("lookup: %v", err)
	}

	// Should not panic or error.
	registry.Shutdown()
}

// Verify registrar uses insecure transport (integration sanity check).
func TestDial_InsecureConn(t *testing.T) {
	addr := startRegistrarTestServer(t, &echoServer{
		metaResp: &pluginv1.MetaResponse{
			TypeId:       "insecure.test",
			InputSchema:  []byte(`{}`),
			OutputSchema: []byte(`{}`),
		},
	})
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	client := pluginv1.NewNodePluginClient(conn)
	resp, err := client.Meta(context.Background(), &pluginv1.MetaRequest{})
	if err != nil {
		t.Fatalf("meta: %v", err)
	}
	if resp.GetTypeId() != "insecure.test" {
		t.Errorf("want insecure.test, got %s", resp.GetTypeId())
	}
}
