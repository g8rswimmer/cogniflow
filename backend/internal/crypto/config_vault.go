package crypto

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/g8rswimmer/cogniflow/internal/node"
	"github.com/g8rswimmer/cogniflow/internal/store"
)

const encPrefix = "enc:"

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

// ---- Auth methods — delegate directly; no sensitive data in these types -----

func (v *ConfigVault) CreateOrganization(ctx context.Context, org store.Organization) (store.Organization, error) {
	return v.inner.CreateOrganization(ctx, org)
}
func (v *ConfigVault) GetOrganization(ctx context.Context, id string) (store.Organization, error) {
	return v.inner.GetOrganization(ctx, id)
}
func (v *ConfigVault) ListOrganizations(ctx context.Context) ([]store.Organization, error) {
	return v.inner.ListOrganizations(ctx)
}
func (v *ConfigVault) DeleteOrganization(ctx context.Context, id string) error {
	return v.inner.DeleteOrganization(ctx, id)
}

func (v *ConfigVault) CreateUser(ctx context.Context, u store.User) (store.User, error) {
	return v.inner.CreateUser(ctx, u)
}
func (v *ConfigVault) GetUser(ctx context.Context, id string) (store.User, error) {
	return v.inner.GetUser(ctx, id)
}
func (v *ConfigVault) GetUserByEmail(ctx context.Context, email string) (store.User, error) {
	return v.inner.GetUserByEmail(ctx, email)
}
func (v *ConfigVault) ListUsers(ctx context.Context, orgID string) ([]store.User, error) {
	return v.inner.ListUsers(ctx, orgID)
}
func (v *ConfigVault) UpdateUserRole(ctx context.Context, userID, role string) error {
	return v.inner.UpdateUserRole(ctx, userID, role)
}
func (v *ConfigVault) UpdateUserPermissions(ctx context.Context, userID string, permissions []string) error {
	return v.inner.UpdateUserPermissions(ctx, userID, permissions)
}
func (v *ConfigVault) DeleteUser(ctx context.Context, userID string) error {
	return v.inner.DeleteUser(ctx, userID)
}

func (v *ConfigVault) CreateInvitation(ctx context.Context, inv store.Invitation) (store.Invitation, error) {
	return v.inner.CreateInvitation(ctx, inv)
}
func (v *ConfigVault) GetInvitationByToken(ctx context.Context, token string) (store.Invitation, error) {
	return v.inner.GetInvitationByToken(ctx, token)
}
func (v *ConfigVault) AcceptInvitation(ctx context.Context, invID string, now time.Time) error {
	return v.inner.AcceptInvitation(ctx, invID, now)
}

func (v *ConfigVault) UpsertOrgEmailSettings(ctx context.Context, s store.OrgEmailSettings) error {
	if s.SMTPPassword != "" {
		enc, err := v.encryptString(s.SMTPPassword)
		if err != nil {
			return fmt.Errorf("config vault: encrypt smtp_password: %w", err)
		}
		s.SMTPPassword = enc
	}
	return v.inner.UpsertOrgEmailSettings(ctx, s)
}

func (v *ConfigVault) GetOrgEmailSettings(ctx context.Context, orgID string) (store.OrgEmailSettings, error) {
	s, err := v.inner.GetOrgEmailSettings(ctx, orgID)
	if err != nil {
		return store.OrgEmailSettings{}, err
	}
	if s.SMTPPassword != "" {
		plain, err := v.decryptString(s.SMTPPassword)
		if err != nil {
			return store.OrgEmailSettings{}, fmt.Errorf("config vault: decrypt smtp_password: %w", err)
		}
		s.SMTPPassword = plain
	}
	return s, nil
}

func (v *ConfigVault) DeleteOrgEmailSettings(ctx context.Context, orgID string) error {
	return v.inner.DeleteOrgEmailSettings(ctx, orgID)
}

// encryptString encrypts a plain-text string with AES-256-GCM and returns
// "enc:<base64>". Already-encrypted values are returned unchanged.
func (v *ConfigVault) encryptString(plain string) (string, error) {
	if strings.HasPrefix(plain, encPrefix) {
		return plain, nil
	}
	ct, err := v.cipher.Encrypt([]byte(plain))
	if err != nil {
		return "", err
	}
	return encPrefix + base64.StdEncoding.EncodeToString(ct), nil
}

// decryptString decrypts a value produced by encryptString.
// Returns the input unchanged if it does not have the enc: prefix.
func (v *ConfigVault) decryptString(encrypted string) (string, error) {
	if !strings.HasPrefix(encrypted, encPrefix) {
		return encrypted, nil
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(encrypted, encPrefix))
	if err != nil {
		return "", fmt.Errorf("base64 decode: %w", err)
	}
	plain, err := v.cipher.Decrypt(decoded)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// CreateWorkflow encrypts sensitive config values before delegating to the inner store.
// After a successful write, version 1 is snapshotted so the initial state is always
// recoverable, consistent with how PUT snapshots every subsequent save.
func (v *ConfigVault) CreateWorkflow(ctx context.Context, w store.Workflow) (store.Workflow, error) {
	v.encryptNodes(w.Nodes)
	created, err := v.inner.CreateWorkflow(ctx, w)
	if err != nil {
		return store.Workflow{}, err
	}
	if vErr := v.inner.CreateWorkflowVersion(ctx, created); vErr != nil {
		slog.WarnContext(ctx, "config vault: initial workflow version snapshot failed",
			"workflow_id", created.ID, "error", vErr)
	}
	return created, nil
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
// After a successful inner store write, a version snapshot is created with the
// encrypted-at-rest state (before decryption) so that sensitive values survive
// a future restore without re-entry.
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
	updated, err := v.inner.UpdateWorkflow(ctx, w)
	if err != nil {
		return store.Workflow{}, err
	}
	// Snapshot the encrypted-at-rest state as a version. Best-effort: a failure
	// here is logged but does not fail the save. updated still has []byte ciphertexts
	// at this point, which is exactly what CreateWorkflowVersion needs.
	if vErr := v.inner.CreateWorkflowVersion(ctx, updated); vErr != nil {
		slog.WarnContext(ctx, "config vault: workflow version snapshot failed",
			"workflow_id", w.ID, "error", vErr)
	}
	return updated, nil
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

// Workflow Version methods.

// CreateWorkflowVersion delegates directly — the inner store is called with the
// encrypted-at-rest workflow, so no re-encryption is needed here.
func (v *ConfigVault) CreateWorkflowVersion(ctx context.Context, w store.Workflow) error {
	return v.inner.CreateWorkflowVersion(ctx, w)
}

// GetLatestWorkflowVersionNumber delegates directly — version numbers contain no sensitive data.
func (v *ConfigVault) GetLatestWorkflowVersionNumber(ctx context.Context, workflowID string) (*int, error) {
	return v.inner.GetLatestWorkflowVersionNumber(ctx, workflowID)
}

// ListWorkflowVersions delegates directly — summaries contain no sensitive config.
func (v *ConfigVault) ListWorkflowVersions(ctx context.Context, workflowID string) ([]store.WorkflowVersionSummary, error) {
	return v.inner.ListWorkflowVersions(ctx, workflowID)
}

// GetWorkflowVersion returns the version with sensitive config values decrypted.
func (v *ConfigVault) GetWorkflowVersion(ctx context.Context, workflowID string, versionNum int) (store.WorkflowVersion, error) {
	ver, err := v.inner.GetWorkflowVersion(ctx, workflowID, versionNum)
	if err != nil {
		return store.WorkflowVersion{}, err
	}
	if err := v.decryptNodes(ver.Definition.Nodes); err != nil {
		return store.WorkflowVersion{}, fmt.Errorf("config vault: decrypt version %d for workflow %s: %w", versionNum, workflowID, err)
	}
	return ver, nil
}

// DeleteWorkflowVersions delegates directly.
func (v *ConfigVault) DeleteWorkflowVersions(ctx context.Context, workflowID string) error {
	return v.inner.DeleteWorkflowVersions(ctx, workflowID)
}

// RestoreWorkflowVersion restores the workflow to a previous version and returns
// the restored workflow with sensitive config values decrypted.
func (v *ConfigVault) RestoreWorkflowVersion(ctx context.Context, workflowID string, versionNum int) (store.Workflow, error) {
	restored, err := v.inner.RestoreWorkflowVersion(ctx, workflowID, versionNum)
	if err != nil {
		return store.Workflow{}, err
	}
	if err := v.decryptNodes(restored.Nodes); err != nil {
		return store.Workflow{}, fmt.Errorf("config vault: decrypt restored workflow %s: %w", workflowID, err)
	}
	return restored, nil
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
