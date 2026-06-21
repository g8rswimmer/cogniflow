package grader_plugin

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/g8rswimmer/cogniflow/internal/store"
	graderv1 "github.com/g8rswimmer/cogniflow/proto/grader/v1"
)

// echoGraderServer is a minimal in-process gRPC server for tests.
type echoGraderServer struct {
	graderv1.UnimplementedGraderPluginServer
	metaResp  *graderv1.MetaResponse
	gradeResp *graderv1.GradeResponse
	gradeErr  error
}

func (s *echoGraderServer) Meta(_ context.Context, _ *graderv1.MetaRequest) (*graderv1.MetaResponse, error) {
	if s.metaResp != nil {
		return s.metaResp, nil
	}
	return &graderv1.MetaResponse{TypeId: "echo.grader", DisplayName: "Echo Grader"}, nil
}

func (s *echoGraderServer) Grade(_ context.Context, _ *graderv1.GradeRequest) (*graderv1.GradeResponse, error) {
	if s.gradeErr != nil {
		return nil, s.gradeErr
	}
	if s.gradeResp != nil {
		return s.gradeResp, nil
	}
	return &graderv1.GradeResponse{
		Result: &graderv1.GradeResponse_GradeResult{
			GradeResult: &graderv1.GradeResult{Verdict: "pass", Explanation: "ok"},
		},
	}, nil
}

// startTestGraderServer starts a gRPC server and returns a client connection.
func startTestGraderServer(t *testing.T, srv graderv1.GraderPluginServer) *grpc.ClientConn {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	gs := grpc.NewServer()
	graderv1.RegisterGraderPluginServer(gs, srv)
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

func TestGrpcProxy_Close_Nil(t *testing.T) {
	proxy := &grpcProxy{meta: GraderMeta{TypeID: "test"}}
	if err := proxy.Close(); err != nil {
		t.Errorf("Close with nil conn unexpected error: %v", err)
	}
}

func TestGrpcProxy_Close(t *testing.T) {
	conn := startTestGraderServer(t, &echoGraderServer{})
	proxy := &grpcProxy{
		meta:   GraderMeta{TypeID: "echo.grader"},
		client: graderv1.NewGraderPluginClient(conn),
		conn:   conn,
	}
	if err := proxy.Close(); err != nil {
		t.Errorf("Close() unexpected error: %v", err)
	}
}

func TestGrpcProxy_Grade_Pass(t *testing.T) {
	conn := startTestGraderServer(t, &echoGraderServer{})
	proxy := &grpcProxy{
		meta:   GraderMeta{TypeID: "echo.grader"},
		client: graderv1.NewGraderPluginClient(conn),
	}

	result := proxy.grade(context.Background(), map[string]any{"field": "value"}, map[string]any{})
	if result.Verdict != store.VerdictPass {
		t.Errorf("want pass, got %s", result.Verdict)
	}
	if result.Explanation != "ok" {
		t.Errorf("want explanation 'ok', got %q", result.Explanation)
	}
}

func TestGrpcProxy_Grade_WithScore(t *testing.T) {
	score := float64(0.75)
	protoScore := score
	conn := startTestGraderServer(t, &echoGraderServer{
		gradeResp: &graderv1.GradeResponse{
			Result: &graderv1.GradeResponse_GradeResult{
				GradeResult: &graderv1.GradeResult{
					Verdict:     "pass",
					Explanation: "partial",
					Score:       &protoScore,
				},
			},
		},
	})
	proxy := &grpcProxy{
		meta:   GraderMeta{TypeID: "echo.grader"},
		client: graderv1.NewGraderPluginClient(conn),
	}

	result := proxy.grade(context.Background(), map[string]any{}, map[string]any{})
	if result.Score == nil {
		t.Fatal("expected score to be set")
	}
	if *result.Score != 0.75 {
		t.Errorf("want score 0.75, got %v", *result.Score)
	}
}

func TestGrpcProxy_Grade_WithCriteriaResults(t *testing.T) {
	criteriaJSON, _ := json.Marshal([]store.CriterionResult{
		{Criterion: "Is good", Met: true, Explanation: "yes"},
	})
	conn := startTestGraderServer(t, &echoGraderServer{
		gradeResp: &graderv1.GradeResponse{
			Result: &graderv1.GradeResponse_GradeResult{
				GradeResult: &graderv1.GradeResult{
					Verdict:         "pass",
					Explanation:     "1 of 1 met",
					CriteriaResults: criteriaJSON,
				},
			},
		},
	})
	proxy := &grpcProxy{
		meta:   GraderMeta{TypeID: "echo.grader"},
		client: graderv1.NewGraderPluginClient(conn),
	}

	result := proxy.grade(context.Background(), map[string]any{}, map[string]any{})
	if len(result.CriteriaResults) != 1 {
		t.Fatalf("want 1 criteria result, got %d", len(result.CriteriaResults))
	}
	if result.CriteriaResults[0].Criterion != "Is good" {
		t.Errorf("unexpected criterion: %v", result.CriteriaResults[0].Criterion)
	}
}

func TestGrpcProxy_Grade_WithActualValue(t *testing.T) {
	actualJSON, _ := json.Marshal("the actual value")
	conn := startTestGraderServer(t, &echoGraderServer{
		gradeResp: &graderv1.GradeResponse{
			Result: &graderv1.GradeResponse_GradeResult{
				GradeResult: &graderv1.GradeResult{
					Verdict:     "fail",
					Explanation: "wrong",
					ActualValue: actualJSON,
				},
			},
		},
	})
	proxy := &grpcProxy{
		meta:   GraderMeta{TypeID: "echo.grader"},
		client: graderv1.NewGraderPluginClient(conn),
	}

	result := proxy.grade(context.Background(), map[string]any{}, map[string]any{})
	if result.Verdict != store.VerdictFail {
		t.Errorf("want fail, got %s", result.Verdict)
	}
	if result.ActualValue != "the actual value" {
		t.Errorf("unexpected actual value: %v", result.ActualValue)
	}
}

func TestGrpcProxy_Grade_GraderError(t *testing.T) {
	conn := startTestGraderServer(t, &echoGraderServer{
		gradeResp: &graderv1.GradeResponse{
			Result: &graderv1.GradeResponse_Error{
				Error: &graderv1.GraderError{Code: "ERR", Message: "grader failed"},
			},
		},
	})
	proxy := &grpcProxy{
		meta:   GraderMeta{TypeID: "echo.grader"},
		client: graderv1.NewGraderPluginClient(conn),
	}

	result := proxy.grade(context.Background(), map[string]any{}, map[string]any{})
	if result.Verdict != store.VerdictError {
		t.Errorf("want error verdict, got %s", result.Verdict)
	}
}

func TestGrpcProxy_Grade_RPCError(t *testing.T) {
	conn := startTestGraderServer(t, &echoGraderServer{
		gradeErr: errors.New("rpc failure"),
	})
	proxy := &grpcProxy{
		meta:   GraderMeta{TypeID: "echo.grader"},
		client: graderv1.NewGraderPluginClient(conn),
	}

	result := proxy.grade(context.Background(), map[string]any{}, map[string]any{})
	if result.Verdict != store.VerdictError {
		t.Errorf("want error verdict, got %s", result.Verdict)
	}
}

func TestGrpcProxy_Grade_EmptyResponse(t *testing.T) {
	conn := startTestGraderServer(t, &echoGraderServer{
		gradeResp: &graderv1.GradeResponse{},
	})
	proxy := &grpcProxy{
		meta:   GraderMeta{TypeID: "echo.grader"},
		client: graderv1.NewGraderPluginClient(conn),
	}

	result := proxy.grade(context.Background(), map[string]any{}, map[string]any{})
	if result.Verdict != store.VerdictError {
		t.Errorf("want error verdict for empty response, got %s", result.Verdict)
	}
}

func TestGrpcProxy_Grade_WithDeadline(t *testing.T) {
	conn := startTestGraderServer(t, &echoGraderServer{})
	proxy := &grpcProxy{
		meta:   GraderMeta{TypeID: "echo.grader"},
		client: graderv1.NewGraderPluginClient(conn),
	}

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(5*time.Second))
	defer cancel()

	result := proxy.grade(ctx, map[string]any{}, map[string]any{})
	if result.Verdict != store.VerdictPass {
		t.Errorf("want pass, got %s: %s", result.Verdict, result.Explanation)
	}
}

func TestPluginGrader_Grade(t *testing.T) {
	conn := startTestGraderServer(t, &echoGraderServer{})
	proxy := &grpcProxy{
		meta:   GraderMeta{TypeID: "echo.grader"},
		client: graderv1.NewGraderPluginClient(conn),
	}
	g := &PluginGrader{proxy: proxy, config: map[string]any{"threshold": 0.8}}

	result := g.Grade(context.Background(), map[string]any{"score": 1.0})
	if result.Verdict != store.VerdictPass {
		t.Errorf("want pass, got %s", result.Verdict)
	}
}
