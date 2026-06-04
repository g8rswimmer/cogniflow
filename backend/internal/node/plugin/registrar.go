package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/g8rswimmer/cogniflow/internal/node"
	"github.com/g8rswimmer/cogniflow/internal/store"
	pluginv1 "github.com/g8rswimmer/cogniflow/proto/plugin/v1"
)

const dialTimeout = 5 * time.Second

// ErrTypeIDMismatch is returned by UpdateOne when the plugin at the new address
// reports a different type_id than the one being updated.
var ErrTypeIDMismatch = errors.New("plugin type_id mismatch")

// pluginStoreIface is the minimal store interface needed by LoadFromStore.
type pluginStoreIface interface {
	ListPluginRegistrations(ctx context.Context) ([]store.PluginRegistration, error)
}

// Register connects to each address in the comma-separated addresses value,
// retrieves each plugin's metadata, and registers a grpcProxy in the provided
// registry. Addresses that fail or return invalid metadata are logged and
// skipped — built-in nodes are unaffected.
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

// RegisterOne dials addr, calls Meta(), registers the proxy in registry, and
// returns a PluginRegistration ready to be persisted by the caller. This is
// the public entry point used by the admin HTTP handler (POST /admin/plugins).
func RegisterOne(ctx context.Context, addr string, registry *node.NodeRegistry) (store.PluginRegistration, error) {
	return dialAndRegister(ctx, addr, registry)
}

// UpdateOne probes newAddr to confirm it serves the expected typeID, then
// atomically swaps the existing registry entry with a fresh connection to
// newAddr. Returns the updated PluginRegistration for the caller to persist.
func UpdateOne(ctx context.Context, typeID, newAddr string, registry *node.NodeRegistry) (store.PluginRegistration, error) {
	// Probe the new address first — before touching the current registration.
	conn, err := grpc.NewClient(newAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return store.PluginRegistration{}, fmt.Errorf("dial %s: %w", newAddr, err)
	}
	defer conn.Close() // probe conn is closed regardless; dialAndRegister opens its own

	probeCtx, probeCancel := context.WithTimeout(ctx, dialTimeout)
	defer probeCancel()

	resp, err := pluginv1.NewNodePluginClient(conn).Meta(probeCtx, &pluginv1.MetaRequest{})
	if err != nil {
		return store.PluginRegistration{}, fmt.Errorf("probe meta rpc %s: %w", newAddr, err)
	}
	if resp.GetTypeId() != typeID {
		return store.PluginRegistration{}, fmt.Errorf(
			"%w: want %q, plugin at %s returned %q", ErrTypeIDMismatch, typeID, newAddr, resp.GetTypeId())
	}

	// Probe succeeded; unregister the old entry (ignore if it was never loaded,
	// e.g. the plugin was unreachable at the last startup).
	_ = registry.Unregister(typeID)

	return dialAndRegister(ctx, newAddr, registry)
}

// LoadFromStore re-establishes gRPC connections for all persisted plugin
// registrations. Call once at startup, before processing PLUGIN_ADDRESSES.
// Unreachable or invalid plugins are logged and skipped — the server starts
// normally and will retry on the next restart.
func LoadFromStore(ctx context.Context, st pluginStoreIface, registry *node.NodeRegistry) {
	regs, err := st.ListPluginRegistrations(ctx)
	if err != nil {
		slog.Warn("plugin registrar: failed to list stored registrations", "error", err)
		return
	}
	for _, reg := range regs {
		if err := loadStored(ctx, reg, registry); err != nil {
			slog.Warn("stored plugin registration failed; skipping until next restart",
				"type_id", reg.TypeID, "address", reg.Address, "error", err)
		}
	}
}

// registerOne is the internal env-var path. It uses a fresh background context
// per plugin so that slow or failing plugins do not deplete the shared startup
// budget. Caller logs the warning on error.
func registerOne(ctx context.Context, addr string, registry *node.NodeRegistry) error {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("dial %s: %w", addr, err)
	}

	committed := false
	defer func() {
		if !committed {
			conn.Close()
		}
	}()

	client := pluginv1.NewNodePluginClient(conn)

	// Fresh independent timeout so slow plugins earlier in the list do not
	// shrink the budget for later ones.
	metaCtx, metaCancel := context.WithTimeout(context.Background(), dialTimeout)
	defer metaCancel()
	stopEarlyCancel := context.AfterFunc(ctx, metaCancel)
	defer stopEarlyCancel()

	resp, err := client.Meta(metaCtx, &pluginv1.MetaRequest{})
	if err != nil {
		return fmt.Errorf("meta rpc %s: %w", addr, err)
	}

	meta, err := protoToMeta(resp)
	if err != nil {
		return fmt.Errorf("invalid meta from %s: %w", addr, err)
	}

	proxy := &grpcProxy{meta: meta, client: client, conn: conn}
	if err := registry.TryRegister(proxy); err != nil {
		return fmt.Errorf("plugin %s: %w", addr, err)
	}
	committed = true
	slog.Info("plugin registered", "type_id", meta.TypeID, "address", addr)
	return nil
}

// dialAndRegister is the shared core used by RegisterOne and UpdateOne.
// ctx is used directly for the Meta() timeout — callers are responsible for
// scoping it appropriately (e.g. an HTTP request context).
func dialAndRegister(ctx context.Context, addr string, registry *node.NodeRegistry) (store.PluginRegistration, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return store.PluginRegistration{}, fmt.Errorf("dial %s: %w", addr, err)
	}

	committed := false
	defer func() {
		if !committed {
			conn.Close()
		}
	}()

	metaCtx, metaCancel := context.WithTimeout(ctx, dialTimeout)
	defer metaCancel()

	grpcClient := pluginv1.NewNodePluginClient(conn)
	resp, err := grpcClient.Meta(metaCtx, &pluginv1.MetaRequest{})
	if err != nil {
		return store.PluginRegistration{}, fmt.Errorf("meta rpc %s: %w", addr, err)
	}

	meta, err := protoToMeta(resp)
	if err != nil {
		return store.PluginRegistration{}, fmt.Errorf("invalid meta from %s: %w", addr, err)
	}

	proxy := &grpcProxy{meta: meta, client: grpcClient, conn: conn}
	if err := registry.TryRegister(proxy); err != nil {
		return store.PluginRegistration{}, err
	}
	committed = true

	return store.PluginRegistration{
		TypeID:       meta.TypeID,
		Address:      addr,
		DisplayName:  meta.DisplayName,
		Category:     meta.Category,
		Description:  meta.Description,
		InputSchema:  meta.InputSchema,
		OutputSchema: meta.OutputSchema,
	}, nil
}

// loadStored re-registers a single persisted plugin at startup using stored
// metadata (so display_name/description changes in the plugin binary don't
// silently alter the registered node type between restarts).
func loadStored(ctx context.Context, reg store.PluginRegistration, registry *node.NodeRegistry) error {
	conn, err := grpc.NewClient(reg.Address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("dial %s: %w", reg.Address, err)
	}

	committed := false
	defer func() {
		if !committed {
			conn.Close()
		}
	}()

	// Fresh independent timeout so one slow plugin does not deplete the shared
	// startup context budget.
	metaCtx, metaCancel := context.WithTimeout(context.Background(), dialTimeout)
	defer metaCancel()
	stopEarlyCancel := context.AfterFunc(ctx, metaCancel)
	defer stopEarlyCancel()

	resp, err := pluginv1.NewNodePluginClient(conn).Meta(metaCtx, &pluginv1.MetaRequest{})
	if err != nil {
		return fmt.Errorf("meta rpc: %w", err)
	}
	if resp.GetTypeId() != reg.TypeID {
		return fmt.Errorf("type_id mismatch: stored %q, plugin returned %q", reg.TypeID, resp.GetTypeId())
	}

	proxy := &grpcProxy{
		meta: node.NodeMeta{
			TypeID:       reg.TypeID,
			DisplayName:  reg.DisplayName,
			Category:     reg.Category,
			Description:  reg.Description,
			InputSchema:  reg.InputSchema,
			OutputSchema: reg.OutputSchema,
		},
		client: pluginv1.NewNodePluginClient(conn),
		conn:   conn,
	}
	if err := registry.TryRegister(proxy); err != nil {
		return fmt.Errorf("register: %w", err)
	}
	committed = true
	slog.Info("stored plugin registered", "type_id", reg.TypeID, "address", reg.Address)
	return nil
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
