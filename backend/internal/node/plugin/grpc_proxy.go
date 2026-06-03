package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"google.golang.org/grpc"

	"github.com/g8rswimmer/cogniflow/internal/node"
	pluginv1 "github.com/g8rswimmer/cogniflow/proto/plugin/v1"
)

// grpcProxy adapts a remote gRPC plugin to the local NodeHandler interface.
type grpcProxy struct {
	meta   node.NodeMeta
	client pluginv1.NodePluginClient
	conn   *grpc.ClientConn
}

func (p *grpcProxy) Meta() node.NodeMeta {
	return p.meta
}

// Close shuts down the underlying gRPC connection. NodeRegistry.Shutdown
// calls this for any registered handler that implements io.Closer.
func (p *grpcProxy) Close() error {
	return p.conn.Close()
}

func (p *grpcProxy) Execute(ctx context.Context, input node.NodeInput) (node.NodeOutput, error) {
	upstreamJSON, err := json.Marshal(input.UpstreamData)
	if err != nil {
		return node.NodeOutput{}, fmt.Errorf("plugin %s: marshal upstream data: %w", p.meta.TypeID, err)
	}
	configJSON, err := json.Marshal(input.Config)
	if err != nil {
		return node.NodeOutput{}, fmt.Errorf("plugin %s: marshal config: %w", p.meta.TypeID, err)
	}

	var timeoutMs int64
	if deadline, ok := ctx.Deadline(); ok {
		timeoutMs = time.Until(deadline).Milliseconds()
	}

	resp, err := p.client.Execute(ctx, &pluginv1.ExecuteRequest{
		UpstreamData:        upstreamJSON,
		Config:              configJSON,
		TimeoutMs:           timeoutMs,
		DirectPredecessorIds: input.DirectPredecessorIDs,
	})
	if err != nil {
		return node.NodeOutput{}, fmt.Errorf("plugin %s: execute rpc: %w", p.meta.TypeID, err)
	}

	if e := resp.GetError(); e != nil {
		return node.NodeOutput{}, fmt.Errorf("[%s] %s", e.GetCode(), e.GetMessage())
	}

	rawData := resp.GetData()
	if len(rawData) == 0 {
		return node.NodeOutput{}, fmt.Errorf("plugin %s: response has neither data nor error set", p.meta.TypeID)
	}

	var data map[string]any
	if err := json.Unmarshal(rawData, &data); err != nil {
		return node.NodeOutput{}, fmt.Errorf("plugin %s: unmarshal response: %w", p.meta.TypeID, err)
	}
	return node.NodeOutput{Data: data}, nil
}
