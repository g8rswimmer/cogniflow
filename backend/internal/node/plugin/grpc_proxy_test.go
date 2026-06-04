package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/g8rswimmer/cogniflow/internal/node"
	pluginv1 "github.com/g8rswimmer/cogniflow/proto/plugin/v1"
)

// echoServer is a minimal in-process gRPC server used by tests.
type echoServer struct {
	pluginv1.UnimplementedNodePluginServer
	metaResp    *pluginv1.MetaResponse
	executeResp *pluginv1.ExecuteResponse
	executeErr  error
}

func (s *echoServer) Meta(_ context.Context, _ *pluginv1.MetaRequest) (*pluginv1.MetaResponse, error) {
	return s.metaResp, nil
}

func (s *echoServer) Execute(_ context.Context, req *pluginv1.ExecuteRequest) (*pluginv1.ExecuteResponse, error) {
	if s.executeErr != nil {
		return nil, s.executeErr
	}
	if s.executeResp != nil {
		return s.executeResp, nil
	}
	// Default: echo upstream_data back as output.
	return &pluginv1.ExecuteResponse{
		Result: &pluginv1.ExecuteResponse_Data{Data: req.GetUpstreamData()},
	}, nil
}

// startTestServer starts a gRPC server with the given handler and returns
// the client connection and a cleanup function.
func startTestServer(t *testing.T, srv pluginv1.NodePluginServer) *grpc.ClientConn {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	gs := grpc.NewServer()
	pluginv1.RegisterNodePluginServer(gs, srv)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.GracefulStop)

	conn, err := grpc.NewClient(lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial test server: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func TestGrpcProxy_Meta(t *testing.T) {
	conn := startTestServer(t, &echoServer{
		metaResp: &pluginv1.MetaResponse{
			TypeId:       "echo.test",
			DisplayName:  "Echo Test",
			Category:     "plugin",
			Description:  "test node",
			InputSchema:  []byte(`{}`),
			OutputSchema: []byte(`{}`),
		},
	})

	proxy := &grpcProxy{
		meta: node.NodeMeta{
			TypeID:      "echo.test",
			DisplayName: "Echo Test",
			Category:    "plugin",
		},
		client: pluginv1.NewNodePluginClient(conn),
		conn:   nil,
	}

	meta := proxy.Meta()
	if meta.TypeID != "echo.test" {
		t.Errorf("want type_id echo.test, got %s", meta.TypeID)
	}
	if meta.Category != "plugin" {
		t.Errorf("want category plugin, got %s", meta.Category)
	}
}

func TestGrpcProxy_Execute_Success(t *testing.T) {
	upstream := map[string]any{"_initial": map[string]any{"key": "value"}}
	upstreamJSON, _ := json.Marshal(upstream)

	conn := startTestServer(t, &echoServer{
		executeResp: &pluginv1.ExecuteResponse{
			Result: &pluginv1.ExecuteResponse_Data{Data: []byte(`{"echoed":true}`)},
		},
	})

	proxy := &grpcProxy{
		meta:   node.NodeMeta{TypeID: "echo.test"},
		client: pluginv1.NewNodePluginClient(conn),
	}

	out, err := proxy.Execute(context.Background(), node.NodeInput{
		UpstreamData: upstream,
		Config:       map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Data["echoed"] != true {
		t.Errorf("want echoed=true, got %v", out.Data["echoed"])
	}
	_ = upstreamJSON
}

func TestGrpcProxy_Execute_PluginError(t *testing.T) {
	conn := startTestServer(t, &echoServer{
		executeResp: &pluginv1.ExecuteResponse{
			Result: &pluginv1.ExecuteResponse_Error{
				Error: &pluginv1.PluginError{Code: "SOME_CODE", Message: "plugin failed"},
			},
		},
	})

	proxy := &grpcProxy{
		meta:   node.NodeMeta{TypeID: "echo.test"},
		client: pluginv1.NewNodePluginClient(conn),
	}

	_, err := proxy.Execute(context.Background(), node.NodeInput{
		UpstreamData: map[string]any{},
		Config:       map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error from plugin error response")
	}
}

func TestGrpcProxy_Execute_RPCError(t *testing.T) {
	conn := startTestServer(t, &echoServer{
		executeErr: errors.New("internal server error"),
	})

	proxy := &grpcProxy{
		meta:   node.NodeMeta{TypeID: "echo.test"},
		client: pluginv1.NewNodePluginClient(conn),
	}

	_, err := proxy.Execute(context.Background(), node.NodeInput{
		UpstreamData: map[string]any{},
		Config:       map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error from rpc failure")
	}
}

func TestGrpcProxy_Execute_EmptyOneof(t *testing.T) {
	conn := startTestServer(t, &echoServer{
		// executeResp is nil — the server returns &ExecuteResponse{} with neither
		// data nor error set, which is what UnimplementedNodePluginServer returns.
		executeResp: &pluginv1.ExecuteResponse{},
	})

	proxy := &grpcProxy{
		meta:   node.NodeMeta{TypeID: "echo.test"},
		client: pluginv1.NewNodePluginClient(conn),
	}

	_, err := proxy.Execute(context.Background(), node.NodeInput{
		UpstreamData: map[string]any{},
		Config:       map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error when response has neither data nor error set")
	}
}

func TestGrpcProxy_Execute_ForwardsDirectPredecessorIDs(t *testing.T) {
	var capturedReq *pluginv1.ExecuteRequest
	srv := &captureServer{
		resp: &pluginv1.ExecuteResponse{
			Result: &pluginv1.ExecuteResponse_Data{Data: []byte(`{"ok":true}`)},
		},
	}
	conn := startTestServer(t, srv)

	proxy := &grpcProxy{
		meta:   node.NodeMeta{TypeID: "echo.test"},
		client: pluginv1.NewNodePluginClient(conn),
	}

	_, err := proxy.Execute(context.Background(), node.NodeInput{
		UpstreamData:         map[string]any{},
		Config:               map[string]any{},
		DirectPredecessorIDs: []string{"n1", "n2"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	capturedReq = srv.lastReq
	if capturedReq == nil {
		t.Fatal("server did not capture request")
	}
	if len(capturedReq.GetDirectPredecessorIds()) != 2 {
		t.Errorf("want 2 predecessor IDs, got %v", capturedReq.GetDirectPredecessorIds())
	}
}

// captureServer records the last ExecuteRequest it receives.
type captureServer struct {
	pluginv1.UnimplementedNodePluginServer
	resp    *pluginv1.ExecuteResponse
	lastReq *pluginv1.ExecuteRequest
}

func (s *captureServer) Meta(_ context.Context, _ *pluginv1.MetaRequest) (*pluginv1.MetaResponse, error) {
	return &pluginv1.MetaResponse{TypeId: "capture.test"}, nil
}

func (s *captureServer) Execute(_ context.Context, req *pluginv1.ExecuteRequest) (*pluginv1.ExecuteResponse, error) {
	s.lastReq = req
	return s.resp, nil
}

func TestGrpcProxy_Close(t *testing.T) {
	conn := startTestServer(t, &echoServer{
		metaResp: &pluginv1.MetaResponse{TypeId: "close.test"},
	})
	proxy := &grpcProxy{
		meta:   node.NodeMeta{TypeID: "close.test"},
		client: pluginv1.NewNodePluginClient(conn),
		conn:   conn,
	}
	if err := proxy.Close(); err != nil {
		t.Errorf("Close() unexpected error: %v", err)
	}
}

func TestGrpcProxy_Execute_WithDeadline(t *testing.T) {
	conn := startTestServer(t, &echoServer{
		executeResp: &pluginv1.ExecuteResponse{
			Result: &pluginv1.ExecuteResponse_Data{Data: []byte(`{"ok":true}`)},
		},
	})

	proxy := &grpcProxy{
		meta:   node.NodeMeta{TypeID: "echo.test"},
		client: pluginv1.NewNodePluginClient(conn),
	}

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(5*time.Second))
	defer cancel()

	out, err := proxy.Execute(ctx, node.NodeInput{
		UpstreamData: map[string]any{},
		Config:       map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Data["ok"] != true {
		t.Errorf("want ok=true, got %v", out.Data["ok"])
	}
}
