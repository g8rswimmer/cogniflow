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
// Returns an error if any ciphertext fails to decrypt so callers never receive
// a silently-corrupted workflow with empty strings in place of secrets.
func (v *ConfigVault) GetWorkflow(ctx context.Context, id string) (store.Workflow, error) {
	w, err := v.inner.GetWorkflow(ctx, id)
	if err != nil {
		return store.Workflow{}, err
	}
	if err := v.decryptNodes(w.Nodes); err != nil {
		return store.Workflow{}, fmt.Errorf("config vault: decrypt workflow %s: %w", id, err)
	}
	return w, nil
}

// ListWorkflows delegates directly — summaries contain no sensitive config.
func (v *ConfigVault) ListWorkflows(ctx context.Context) ([]store.WorkflowSummary, error) {
	return v.inner.ListWorkflows(ctx)
}

// UpdateWorkflow encrypts sensitive config values before delegating.
// If a sensitive field arrives with the masked sentinel "***", the existing
// encrypted ciphertext is fetched from the inner store and preserved so that
// a re-save without re-entering the key does not overwrite it with garbage.
func (v *ConfigVault) UpdateWorkflow(ctx context.Context, w store.Workflow) (store.Workflow, error) {
	existing, err := v.inner.GetWorkflow(ctx, w.ID)
	if err != nil {
		return store.Workflow{}, err
	}
	existingByID := make(map[string]*store.WorkflowNode, len(existing.Nodes))
	for i := range existing.Nodes {
		existingByID[existing.Nodes[i].ID] = &existing.Nodes[i]
	}
	v.encryptNodesPreserving(w.Nodes, existingByID)
	return v.inner.UpdateWorkflow(ctx, w)
}

// encryptNodesPreserving encrypts like encryptNodes but, for any sensitive
// field whose incoming value is the masked sentinel "***", copies the existing
// raw ciphertext from the stored node instead of re-encrypting.
func (v *ConfigVault) encryptNodesPreserving(nodes []store.WorkflowNode, existing map[string]*store.WorkflowNode) {
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
			// Sentinel: frontend is telling us "don't change this field".
			if strVal == "***" {
				if prev, found := existing[n.ID]; found {
					if raw, hasKey := prev.Config[key]; hasKey {
						if ct, isCT := raw.([]byte); isCT {
							n.Config[key] = ct
							n.SensitiveKeys[key] = true
							continue
						}
					}
				}
				// No prior value found (new node) — drop the sentinel so we don't store garbage.
				slog.Warn("config vault: sentinel '***' on node with no prior ciphertext; field dropped",
					"node", n.ID, "key", key)
				delete(n.Config, key)
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

// DeleteWorkflow delegates directly.
func (v *ConfigVault) DeleteWorkflow(ctx context.Context, id string) error {
	return v.inner.DeleteWorkflow(ctx, id)
}

func (v *ConfigVault) GetWorkflowSchema(ctx context.Context, id string) (json.RawMessage, error) {
	return v.inner.GetWorkflowSchema(ctx, id)
}

// Run methods delegate directly — runs do not store sensitive config.

func (v *ConfigVault) CreateRun(ctx context.Context, r store.Run) (store.Run, error) {
	return v.inner.CreateRun(ctx, r)
}

func (v *ConfigVault) UpdateRunStatus(ctx context.Context, runID string, status store.RunStatus, output map[string]any) error {
	return v.inner.UpdateRunStatus(ctx, runID, status, output)
}

func (v *ConfigVault) SaveRunNodeResults(ctx context.Context, runID string, results map[string]store.NodeResult) error {
	return v.inner.SaveRunNodeResults(ctx, runID, results)
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

// Plugin registration methods — no sensitive fields; delegate directly.

func (v *ConfigVault) SavePluginRegistration(ctx context.Context, reg store.PluginRegistration) error {
	return v.inner.SavePluginRegistration(ctx, reg)
}

func (v *ConfigVault) GetPluginRegistration(ctx context.Context, typeID string) (store.PluginRegistration, error) {
	return v.inner.GetPluginRegistration(ctx, typeID)
}

func (v *ConfigVault) ListPluginRegistrations(ctx context.Context) ([]store.PluginRegistration, error) {
	return v.inner.ListPluginRegistrations(ctx)
}

func (v *ConfigVault) DeletePluginRegistration(ctx context.Context, typeID string) error {
	return v.inner.DeletePluginRegistration(ctx, typeID)
}

// Grader plugin registration methods — no sensitive fields; delegate directly.

func (v *ConfigVault) SaveGraderRegistration(ctx context.Context, reg store.GraderRegistration) error {
	return v.inner.SaveGraderRegistration(ctx, reg)
}
func (v *ConfigVault) GetGraderRegistration(ctx context.Context, typeID string) (store.GraderRegistration, error) {
	return v.inner.GetGraderRegistration(ctx, typeID)
}
func (v *ConfigVault) ListGraderRegistrations(ctx context.Context) ([]store.GraderRegistration, error) {
	return v.inner.ListGraderRegistrations(ctx)
}
func (v *ConfigVault) DeleteGraderRegistration(ctx context.Context, typeID string) error {
	return v.inner.DeleteGraderRegistration(ctx, typeID)
}

// Eval methods — no sensitive fields; delegate directly.

func (v *ConfigVault) CreateEvalSuite(ctx context.Context, s store.EvalSuite) (store.EvalSuite, error) {
	return v.inner.CreateEvalSuite(ctx, s)
}
func (v *ConfigVault) GetEvalSuite(ctx context.Context, id string) (store.EvalSuite, error) {
	return v.inner.GetEvalSuite(ctx, id)
}
func (v *ConfigVault) ListEvalSuites(ctx context.Context, workflowID string) ([]store.EvalSuiteSummary, error) {
	return v.inner.ListEvalSuites(ctx, workflowID)
}
func (v *ConfigVault) ListEvalSuitesByCronTrigger(ctx context.Context) ([]store.EvalSuite, error) {
	return v.inner.ListEvalSuitesByCronTrigger(ctx)
}
func (v *ConfigVault) UpdateEvalSuite(ctx context.Context, s store.EvalSuite) (store.EvalSuite, error) {
	return v.inner.UpdateEvalSuite(ctx, s)
}
func (v *ConfigVault) DeleteEvalSuite(ctx context.Context, id string) error {
	return v.inner.DeleteEvalSuite(ctx, id)
}

func (v *ConfigVault) CreateTestCase(ctx context.Context, tc store.TestCase) (store.TestCase, error) {
	return v.inner.CreateTestCase(ctx, tc)
}
func (v *ConfigVault) GetTestCase(ctx context.Context, id string) (store.TestCase, error) {
	return v.inner.GetTestCase(ctx, id)
}
func (v *ConfigVault) ListTestCases(ctx context.Context, suiteID string) ([]store.TestCase, error) {
	return v.inner.ListTestCases(ctx, suiteID)
}
func (v *ConfigVault) UpdateTestCase(ctx context.Context, tc store.TestCase) (store.TestCase, error) {
	return v.inner.UpdateTestCase(ctx, tc)
}
func (v *ConfigVault) DeleteTestCase(ctx context.Context, id string) error {
	return v.inner.DeleteTestCase(ctx, id)
}
func (v *ConfigVault) ReorderTestCases(ctx context.Context, suiteID string, orderedIDs []string) error {
	return v.inner.ReorderTestCases(ctx, suiteID, orderedIDs)
}

func (v *ConfigVault) CreateEvalRun(ctx context.Context, r store.EvalRun) (store.EvalRun, error) {
	return v.inner.CreateEvalRun(ctx, r)
}
func (v *ConfigVault) GetEvalRun(ctx context.Context, id string) (store.EvalRun, error) {
	return v.inner.GetEvalRun(ctx, id)
}
func (v *ConfigVault) ListEvalRuns(ctx context.Context, f store.EvalRunFilter) ([]store.EvalRun, error) {
	return v.inner.ListEvalRuns(ctx, f)
}
func (v *ConfigVault) UpdateEvalRunStatus(ctx context.Context, runID string, status store.EvalRunStatus, counts store.EvalRunCounts) error {
	return v.inner.UpdateEvalRunStatus(ctx, runID, status, counts)
}
func (v *ConfigVault) IncrementEvalRunCounts(ctx context.Context, runID string, delta store.EvalRunCounts) error {
	return v.inner.IncrementEvalRunCounts(ctx, runID, delta)
}

func (v *ConfigVault) CreateTestCaseResult(ctx context.Context, r store.TestCaseResult) (store.TestCaseResult, error) {
	return v.inner.CreateTestCaseResult(ctx, r)
}
func (v *ConfigVault) GetTestCaseResult(ctx context.Context, id string) (store.TestCaseResult, error) {
	return v.inner.GetTestCaseResult(ctx, id)
}
func (v *ConfigVault) ListTestCaseResults(ctx context.Context, evalRunID string) ([]store.TestCaseResult, error) {
	return v.inner.ListTestCaseResults(ctx, evalRunID)
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
// are replaced with their plaintext strings. Returns the first decryption error
// so the caller can surface it rather than continuing with a corrupted config.
func (v *ConfigVault) decryptNodes(nodes []store.WorkflowNode) error {
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
				return fmt.Errorf("node %s field %q: %w", n.ID, key, err)
			}
			n.Config[key] = string(plaintext)
		}
	}
	return nil
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
