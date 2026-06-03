package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/g8rswimmer/cogniflow/internal/node"
	pluginv1 "github.com/g8rswimmer/cogniflow/proto/plugin/v1"
)

const dialTimeout = 5 * time.Second

// Register connects to each address in the comma-separated PLUGIN_ADDRESSES
// value, retrieves each plugin's metadata, and registers a grpcProxy in the
// provided registry. Addresses that fail to connect or return invalid metadata
// are logged and skipped — built-in nodes are unaffected.
func Register(ctx context.Context, addresses string, registry *node.NodeRegistry) {
	if addresses == "" {
		return
	}
	for _, addr := range strings.Split(addresses, ",") {
		addr = strings.TrimSpace(addr)
		if addr == "" {
			continue
		}
		if err := registerOne(ctx, addr, registry); err != nil {
			slog.Warn("plugin registration failed", "address", addr, "error", err)
		}
	}
}

func registerOne(ctx context.Context, addr string, registry *node.NodeRegistry) error {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("dial %s: %w", addr, err)
	}

	// closeConn is set to true until we successfully hand the conn to a proxy.
	// Using a success flag avoids fragile duplicate conn.Close() calls on every
	// error path.
	committed := false
	defer func() {
		if !committed {
			conn.Close()
		}
	}()

	client := pluginv1.NewNodePluginClient(conn)

	// Use a fresh independent timeout for the Meta RPC so that slow plugins
	// earlier in the PLUGIN_ADDRESSES list do not shrink the budget available
	// to later plugins in the same registration loop.
	metaCtx, metaCancel := context.WithTimeout(context.Background(), dialTimeout)
	defer metaCancel()
	// Honour cancellation of the caller's context (e.g. server shutdown) while
	// still guaranteeing each plugin a full dialTimeout window.
	metaCtx, metaCancel2 := withEarlyCancel(metaCtx, ctx)
	defer metaCancel2()

	resp, err := client.Meta(metaCtx, &pluginv1.MetaRequest{})
	if err != nil {
		return fmt.Errorf("meta rpc %s: %w", addr, err)
	}

	meta, err := protoToMeta(resp)
	if err != nil {
		return fmt.Errorf("invalid meta from %s: %w", addr, err)
	}

	// Guard against a plugin advertising a TypeID already held by a built-in or
	// another plugin. NodeRegistry.Register panics on collision; returning an
	// error here keeps the server alive.
	if _, lookupErr := registry.Lookup(meta.TypeID); lookupErr == nil {
		return fmt.Errorf("plugin %s: type_id %q already registered; skipping to avoid collision", addr, meta.TypeID)
	}

	proxy := &grpcProxy{meta: meta, client: client, conn: conn}
	registry.Register(proxy)
	committed = true
	slog.Info("plugin registered", "type_id", meta.TypeID, "address", addr)
	return nil
}

// withEarlyCancel returns a context that is cancelled whenever either parent
// is cancelled, using whichever deadline comes first.
func withEarlyCancel(a, b context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(a)
	go func() {
		select {
		case <-b.Done():
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, cancel
}

func protoToMeta(r *pluginv1.MetaResponse) (node.NodeMeta, error) {
	if r.GetTypeId() == "" {
		return node.NodeMeta{}, fmt.Errorf("type_id is empty")
	}

	inputSchema := json.RawMessage(r.GetInputSchema())
	if len(inputSchema) == 0 {
		inputSchema = json.RawMessage(`{}`)
	}
	outputSchema := json.RawMessage(r.GetOutputSchema())
	if len(outputSchema) == 0 {
		outputSchema = json.RawMessage(`{}`)
	}

	// Validate that schemas are valid JSON.
	var tmp any
	if err := json.Unmarshal(inputSchema, &tmp); err != nil {
		return node.NodeMeta{}, fmt.Errorf("invalid input_schema JSON: %w", err)
	}
	if err := json.Unmarshal(outputSchema, &tmp); err != nil {
		return node.NodeMeta{}, fmt.Errorf("invalid output_schema JSON: %w", err)
	}

	return node.NodeMeta{
		TypeID:       r.GetTypeId(),
		DisplayName:  r.GetDisplayName(),
		Category:     "plugin",
		Description:  r.GetDescription(),
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
	}, nil
}
