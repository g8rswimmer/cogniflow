package crypto

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/g8rswimmer/cogniflow/internal/node"
	"github.com/g8rswimmer/cogniflow/internal/store"
)

// Compile-time assertion that *ConfigVault implements store.Store.
var _ store.Store = (*ConfigVault)(nil)

// ConfigVault wraps a Store and transparently encrypts sensitive node config
// values on write and decrypts them on read.
type ConfigVault struct {
	inner    store.Store
	cipher   *Cipher
	registry *node.NodeRegistry
}

// NewConfigVault creates a ConfigVault that decorates the given Store.
func NewConfigVault(inner store.Store, cipher *Cipher, registry *node.NodeRegistry) *ConfigVault {
	return &ConfigVault{inner: inner, cipher: cipher, registry: registry}
}

// CreateWorkflow encrypts sensitive config values before delegating to the inner store.
func (v *ConfigVault) CreateWorkflow(ctx context.Context, w store.Workflow) (store.Workflow, error) {
	v.encryptNodes(w.Nodes)
	return v.inner.CreateWorkflow(ctx, w)
}

// GetWorkflow delegates to the inner store then decrypts sensitive config values.
func (v *ConfigVault) GetWorkflow(ctx context.Context, id string) (store.Workflow, error) {
	w, err := v.inner.GetWorkflow(ctx, id)
	if err != nil {
		return store.Workflow{}, err
	}
	v.decryptNodes(w.Nodes)
	return w, nil
}

// ListWorkflows delegates directly — summaries contain no sensitive config.
func (v *ConfigVault) ListWorkflows(ctx context.Context) ([]store.WorkflowSummary, error) {
	return v.inner.ListWorkflows(ctx)
}

// UpdateWorkflow encrypts sensitive config values before delegating.
func (v *ConfigVault) UpdateWorkflow(ctx context.Context, w store.Workflow) (store.Workflow, error) {
	v.encryptNodes(w.Nodes)
	return v.inner.UpdateWorkflow(ctx, w)
}

// DeleteWorkflow delegates directly.
func (v *ConfigVault) DeleteWorkflow(ctx context.Context, id string) error {
	return v.inner.DeleteWorkflow(ctx, id)
}

// Run methods delegate directly — runs do not store sensitive config.

func (v *ConfigVault) CreateRun(ctx context.Context, r store.Run) (store.Run, error) {
	return v.inner.CreateRun(ctx, r)
}

func (v *ConfigVault) UpdateRunStatus(ctx context.Context, runID string, status store.RunStatus, output map[string]any) error {
	return v.inner.UpdateRunStatus(ctx, runID, status, output)
}

func (v *ConfigVault) GetRun(ctx context.Context, runID string) (store.Run, error) {
	return v.inner.GetRun(ctx, runID)
}

func (v *ConfigVault) ListRuns(ctx context.Context, f store.RunFilter) ([]store.Run, error) {
	return v.inner.ListRuns(ctx, f)
}

func (v *ConfigVault) UpsertChunks(ctx context.Context, chunks []store.RAGChunk) error {
	return v.inner.UpsertChunks(ctx, chunks)
}

func (v *ConfigVault) SearchChunks(ctx context.Context, embedding []float32, topK int, docFilter string) ([]store.RAGChunkResult, error) {
	return v.inner.SearchChunks(ctx, embedding, topK, docFilter)
}

func (v *ConfigVault) SaveTriggerConfig(ctx context.Context, workflowID string, cfg store.TriggerConfig) error {
	return v.inner.SaveTriggerConfig(ctx, workflowID, cfg)
}

func (v *ConfigVault) GetTriggerConfig(ctx context.Context, workflowID string) (store.TriggerConfig, error) {
	return v.inner.GetTriggerConfig(ctx, workflowID)
}

func (v *ConfigVault) ListTriggerConfigs(ctx context.Context) ([]store.WorkflowTrigger, error) {
	return v.inner.ListTriggerConfigs(ctx)
}

// encryptNodes mutates nodes in place: sensitive values become ciphertext ([]byte)
// and SensitiveKeys is populated. Unknown node types are stored unencrypted.
func (v *ConfigVault) encryptNodes(nodes []store.WorkflowNode) {
	for i := range nodes {
		n := &nodes[i]
		if n.Config == nil {
			continue
		}
		sensitiveKeys := v.sensitiveKeys(n.TypeID)
		if n.SensitiveKeys == nil {
			n.SensitiveKeys = make(map[string]bool, len(sensitiveKeys))
		}
		for _, key := range sensitiveKeys {
			val, ok := n.Config[key]
			if !ok {
				continue
			}
			strVal, ok := val.(string)
			if !ok {
				slog.Warn("config vault: sensitive value is not a string, skipping encryption",
					"node", n.ID, "key", key, "type", fmt.Sprintf("%T", val))
				continue
			}
			ciphertext, err := v.cipher.Encrypt([]byte(strVal))
			if err != nil {
				slog.Error("config vault: encrypt failed", "node", n.ID, "key", key, "error", err)
				continue
			}
			n.Config[key] = ciphertext
			n.SensitiveKeys[key] = true
		}
	}
}

// decryptNodes mutates nodes in place: []byte ciphertext values in SensitiveKeys
// are replaced with their plaintext strings.
func (v *ConfigVault) decryptNodes(nodes []store.WorkflowNode) {
	for i := range nodes {
		n := &nodes[i]
		for key, isSensitive := range n.SensitiveKeys {
			if !isSensitive {
				continue
			}
			raw, ok := n.Config[key]
			if !ok {
				continue
			}
			ciphertext, ok := raw.([]byte)
			if !ok {
				continue
			}
			plaintext, err := v.cipher.Decrypt(ciphertext)
			if err != nil {
				slog.Error("config vault: decrypt failed", "node", n.ID, "key", key, "error", err)
				n.Config[key] = ""
				continue
			}
			n.Config[key] = string(plaintext)
		}
	}
}

// sensitiveKeys returns the list of config keys marked x-sensitive:true in the
// node type's input schema.
func (v *ConfigVault) sensitiveKeys(typeID string) []string {
	h, err := v.registry.Lookup(typeID)
	if err != nil {
		return nil
	}
	return parseSensitiveKeys(h.Meta().InputSchema)
}

func parseSensitiveKeys(schema json.RawMessage) []string {
	var s struct {
		Properties map[string]struct {
			XSensitive bool `json:"x-sensitive"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(schema, &s); err != nil {
		return nil
	}
	var keys []string
	for k, prop := range s.Properties {
		if prop.XSensitive {
			keys = append(keys, k)
		}
	}
	return keys
}
