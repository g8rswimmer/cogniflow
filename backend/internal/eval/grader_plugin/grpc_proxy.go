package grader_plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"google.golang.org/grpc"

	"github.com/g8rswimmer/cogniflow/internal/store"
	graderv1 "github.com/g8rswimmer/cogniflow/proto/grader/v1"
)

// grpcProxy adapts a remote gRPC grader plugin to the local Grade interface.
type grpcProxy struct {
	meta   GraderMeta
	client graderv1.GraderPluginClient
	conn   *grpc.ClientConn
}

// Close shuts down the underlying gRPC connection.
func (p *grpcProxy) Close() error {
	if p.conn == nil {
		return nil
	}
	return p.conn.Close()
}

// grade forwards the evaluation to the remote plugin and translates the
// response into a store.GraderResult. config is the grader-instance config
// map from the GraderDef.
func (p *grpcProxy) grade(ctx context.Context, data map[string]any, config map[string]any) store.GraderResult {
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return store.GraderResult{
			GraderType:  "plugin",
			Verdict:     store.VerdictError,
			Explanation: fmt.Sprintf("grader plugin %s: marshal data: %v", p.meta.TypeID, err),
		}
	}
	configJSON, err := json.Marshal(config)
	if err != nil {
		return store.GraderResult{
			GraderType:  "plugin",
			Verdict:     store.VerdictError,
			Explanation: fmt.Sprintf("grader plugin %s: marshal config: %v", p.meta.TypeID, err),
		}
	}

	var timeoutMs int64
	if deadline, ok := ctx.Deadline(); ok {
		if d := time.Until(deadline); d > 0 {
			timeoutMs = d.Milliseconds()
		}
	}

	resp, err := p.client.Grade(ctx, &graderv1.GradeRequest{
		Data:      dataJSON,
		Config:    configJSON,
		TimeoutMs: timeoutMs,
	})
	if err != nil {
		return store.GraderResult{
			GraderType:  "plugin",
			Verdict:     store.VerdictError,
			Explanation: fmt.Sprintf("grader plugin %s: grade rpc: %v", p.meta.TypeID, err),
		}
	}

	if e := resp.GetError(); e != nil {
		return store.GraderResult{
			GraderType:  "plugin",
			Verdict:     store.VerdictError,
			Explanation: fmt.Sprintf("[%s] %s", e.GetCode(), e.GetMessage()),
		}
	}

	gr := resp.GetGradeResult()
	if gr == nil {
		return store.GraderResult{
			GraderType:  "plugin",
			Verdict:     store.VerdictError,
			Explanation: fmt.Sprintf("grader plugin %s: response has neither grade_result nor error set", p.meta.TypeID),
		}
	}

	result := store.GraderResult{
		GraderType:  "plugin",
		Verdict:     store.GraderVerdict(gr.GetVerdict()),
		Explanation: gr.GetExplanation(),
	}

	if gr.Score != nil {
		s := gr.GetScore()
		result.Score = &s
	}

	if raw := gr.GetActualValue(); len(raw) > 0 {
		var v any
		if err := json.Unmarshal(raw, &v); err == nil {
			result.ActualValue = v
		}
	}

	if raw := gr.GetCriteriaResults(); len(raw) > 0 {
		var crs []store.CriterionResult
		if err := json.Unmarshal(raw, &crs); err == nil {
			result.CriteriaResults = crs
		}
	}

	return result
}

// PluginGrader wraps a grpcProxy with a bound config and implements the
// eval.Grader interface (Grade(ctx, data) store.GraderResult).
type PluginGrader struct {
	proxy  *grpcProxy
	config map[string]any
}

// Grade implements eval.Grader.
func (g *PluginGrader) Grade(ctx context.Context, data map[string]any) store.GraderResult {
	return g.proxy.grade(ctx, data, g.config)
}
