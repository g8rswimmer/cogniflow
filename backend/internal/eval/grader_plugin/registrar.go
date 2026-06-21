package grader_plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/g8rswimmer/cogniflow/internal/store"
	graderv1 "github.com/g8rswimmer/cogniflow/proto/grader/v1"
)

const dialTimeout = 5 * time.Second

// ErrTypeIDMismatch is returned by UpdateOne when the plugin at the new address
// reports a different type_id than the one being updated.
var ErrTypeIDMismatch = errors.New("grader plugin type_id mismatch")

// graderPluginStoreIface is the minimal store interface needed by LoadFromStore.
type graderPluginStoreIface interface {
	ListGraderRegistrations(ctx context.Context) ([]store.GraderRegistration, error)
}

// RegisterOne dials addr, calls Meta(), registers the proxy in registry, and
// returns a GraderRegistration ready to be persisted by the caller.
func RegisterOne(ctx context.Context, addr string, registry *GraderRegistry) (store.GraderRegistration, error) {
	return dialAndRegister(ctx, addr, "", registry.Register)
}

// UpdateOne dials newAddr, verifies it reports the expected typeID, atomically
// replaces the existing registry entry (closing the old connection outside the
// lock), and returns the updated GraderRegistration for the caller to persist.
// If dialAndRegister fails the existing registry entry is untouched.
func UpdateOne(ctx context.Context, typeID, newAddr string, registry *GraderRegistry) (store.GraderRegistration, error) {
	return dialAndRegister(ctx, newAddr, typeID, func(p *grpcProxy) error {
		registry.Replace(p)
		return nil
	})
}

// LoadFromStore re-establishes gRPC connections for all persisted grader plugin
// registrations concurrently. Call once at startup before accepting traffic.
// Unreachable or invalid plugins are logged and skipped.
func LoadFromStore(ctx context.Context, st graderPluginStoreIface, registry *GraderRegistry) {
	regs, err := st.ListGraderRegistrations(ctx)
	if err != nil {
		slog.Warn("grader plugin registrar: failed to list stored registrations", "error", err)
		return
	}
	var wg sync.WaitGroup
	for _, reg := range regs {
		wg.Add(1)
		go func(reg store.GraderRegistration) {
			defer wg.Done()
			if err := loadStored(ctx, reg, registry); err != nil {
				slog.Warn("stored grader plugin registration failed; skipping until next restart",
					"type_id", reg.TypeID, "address", reg.Address, "error", err)
			}
		}(reg)
	}
	wg.Wait()
}

// dialAndRegister is the shared core: dials addr, calls Meta() under a
// dialTimeout, validates expectedTypeID if non-empty, builds a grpcProxy,
// and calls register(proxy). On success it returns the GraderRegistration;
// on any failure the connection is closed.
func dialAndRegister(ctx context.Context, addr, expectedTypeID string, register func(*grpcProxy) error) (store.GraderRegistration, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return store.GraderRegistration{}, fmt.Errorf("dial %s: %w", addr, err)
	}

	committed := false
	defer func() {
		if !committed {
			conn.Close()
		}
	}()

	metaCtx, metaCancel := context.WithTimeout(ctx, dialTimeout)
	defer metaCancel()

	grpcClient := graderv1.NewGraderPluginClient(conn)
	resp, err := grpcClient.Meta(metaCtx, &graderv1.MetaRequest{})
	if err != nil {
		return store.GraderRegistration{}, fmt.Errorf("meta rpc %s: %w", addr, err)
	}

	meta, err := protoToMeta(resp)
	if err != nil {
		return store.GraderRegistration{}, fmt.Errorf("invalid meta from %s: %w", addr, err)
	}

	if expectedTypeID != "" && meta.TypeID != expectedTypeID {
		return store.GraderRegistration{}, fmt.Errorf(
			"%w: want %q, plugin at %s returned %q", ErrTypeIDMismatch, expectedTypeID, addr, meta.TypeID)
	}

	proxy := &grpcProxy{meta: meta, client: grpcClient, conn: conn}
	if err := register(proxy); err != nil {
		return store.GraderRegistration{}, err
	}
	committed = true

	return store.GraderRegistration{
		TypeID:       meta.TypeID,
		Address:      addr,
		DisplayName:  meta.DisplayName,
		Description:  meta.Description,
		ConfigSchema: json.RawMessage(meta.ConfigSchema),
	}, nil
}

// loadStored re-registers a single persisted grader plugin at startup using
// stored metadata so display_name/description changes in the plugin binary
// don't silently alter the grader type between restarts.
func loadStored(ctx context.Context, reg store.GraderRegistration, registry *GraderRegistry) error {
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

	metaCtx, metaCancel := context.WithTimeout(context.Background(), dialTimeout)
	defer metaCancel()
	stopEarlyCancel := context.AfterFunc(ctx, metaCancel)
	defer stopEarlyCancel()

	client := graderv1.NewGraderPluginClient(conn)
	resp, err := client.Meta(metaCtx, &graderv1.MetaRequest{})
	if err != nil {
		return fmt.Errorf("meta rpc: %w", err)
	}
	if resp.GetTypeId() != reg.TypeID {
		return fmt.Errorf("type_id mismatch: stored %q, plugin returned %q", reg.TypeID, resp.GetTypeId())
	}

	proxy := &grpcProxy{
		meta: GraderMeta{
			TypeID:       reg.TypeID,
			DisplayName:  reg.DisplayName,
			Description:  reg.Description,
			ConfigSchema: []byte(reg.ConfigSchema),
		},
		client: client,
		conn:   conn,
	}
	if err := registry.Register(proxy); err != nil {
		return fmt.Errorf("register: %w", err)
	}
	committed = true
	slog.Info("stored grader plugin registered", "type_id", reg.TypeID, "address", reg.Address)
	return nil
}

// protoToMeta converts a MetaResponse to a GraderMeta, validating required fields.
func protoToMeta(r *graderv1.MetaResponse) (GraderMeta, error) {
	if r.GetTypeId() == "" {
		return GraderMeta{}, fmt.Errorf("type_id is empty")
	}

	configSchema := r.GetConfigSchema()
	if len(configSchema) == 0 {
		configSchema = []byte(`{}`)
	}

	var tmp any
	if err := json.Unmarshal(configSchema, &tmp); err != nil {
		return GraderMeta{}, fmt.Errorf("invalid config_schema JSON: %w", err)
	}

	return GraderMeta{
		TypeID:       r.GetTypeId(),
		DisplayName:  r.GetDisplayName(),
		Description:  r.GetDescription(),
		ConfigSchema: configSchema,
	}, nil
}

// NewPluginGrader constructs a PluginGrader from the registry for the given
// type_id and config. Returns an error if the type_id is not registered.
func NewPluginGrader(registry *GraderRegistry, typeID string, config map[string]any) (*PluginGrader, error) {
	proxy, ok := registry.Get(typeID)
	if !ok {
		return nil, fmt.Errorf("grader plugin %q: not registered", typeID)
	}
	return &PluginGrader{proxy: proxy, config: config}, nil
}
