package eval

import (
	"testing"

	"github.com/g8rswimmer/cogniflow/internal/crypto"
	"github.com/g8rswimmer/cogniflow/internal/store"
)

func newTestVault(t *testing.T) *GraderVault {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	c, err := crypto.NewCipher(key)
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	return NewGraderVault(c)
}

func TestGraderVault_EncryptDecrypt_LLMJudge(t *testing.T) {
	v := newTestVault(t)
	graders := []store.GraderDef{
		{
			ID: "g1", Type: "llm_judge", Scope: "workflow",
			Config: map[string]any{"api_key": "sk-secret", "model": "gpt-4o"},
		},
	}

	encrypted, err := v.EncryptGraders(graders)
	if err != nil {
		t.Fatalf("EncryptGraders: %v", err)
	}
	encKey, _ := encrypted[0].Config["api_key"].(string)
	if encKey == "sk-secret" {
		t.Error("api_key should be encrypted, not plaintext")
	}
	if len(encKey) == 0 {
		t.Error("encrypted api_key should be non-empty")
	}

	decrypted, err := v.DecryptGraders(encrypted)
	if err != nil {
		t.Fatalf("DecryptGraders: %v", err)
	}
	decKey, _ := decrypted[0].Config["api_key"].(string)
	if decKey != "sk-secret" {
		t.Errorf("decrypted api_key: want sk-secret, got %q", decKey)
	}
	// other config fields unchanged
	if decrypted[0].Config["model"] != "gpt-4o" {
		t.Error("model field should be preserved")
	}
}

func TestGraderVault_EncryptDecrypt_Checklist(t *testing.T) {
	v := newTestVault(t)
	graders := []store.GraderDef{
		{ID: "g1", Type: "checklist", Config: map[string]any{"api_key": "an-key"}},
	}
	enc, err := v.EncryptGraders(graders)
	if err != nil {
		t.Fatalf("EncryptGraders: %v", err)
	}
	dec, err := v.DecryptGraders(enc)
	if err != nil {
		t.Fatalf("DecryptGraders: %v", err)
	}
	if dec[0].Config["api_key"] != "an-key" {
		t.Errorf("round-trip failed: got %q", dec[0].Config["api_key"])
	}
}

func TestGraderVault_NonSensitiveTypes_Unchanged(t *testing.T) {
	v := newTestVault(t)
	graders := []store.GraderDef{
		{ID: "g1", Type: "string_match", Config: map[string]any{"field_path": "x"}},
		{ID: "g2", Type: "numeric_threshold", Config: map[string]any{"threshold": 100}},
		{ID: "g3", Type: "json_schema", Config: map[string]any{"schema": map[string]any{"type": "object"}}},
	}
	enc, err := v.EncryptGraders(graders)
	if err != nil {
		t.Fatalf("EncryptGraders: %v", err)
	}
	// configs should be unchanged — check that no keys were removed or added
	for i, g := range enc {
		if len(g.Config) != len(graders[i].Config) {
			t.Errorf("grader[%d] config length changed: want %d, got %d", i, len(graders[i].Config), len(g.Config))
		}
		for k := range graders[i].Config {
			if _, ok := g.Config[k]; !ok {
				t.Errorf("grader[%d] config key %q was removed", i, k)
			}
		}
	}
}

func TestGraderVault_MaskGraders(t *testing.T) {
	v := newTestVault(t)
	graders := []store.GraderDef{
		{ID: "g1", Type: "llm_judge", Config: map[string]any{"api_key": "enc:abc123", "model": "gpt-4o"}},
		{ID: "g2", Type: "string_match", Config: map[string]any{"field_path": "x"}},
	}
	masked := v.MaskGraders(graders)

	if masked[0].Config["api_key"] != "***" {
		t.Errorf("llm_judge api_key should be masked, got %q", masked[0].Config["api_key"])
	}
	if masked[0].Config["model"] != "gpt-4o" {
		t.Error("non-sensitive field should be preserved")
	}
	if _, ok := masked[1].Config["api_key"]; ok {
		t.Error("string_match should not have api_key added")
	}
}

func TestGraderVault_Encrypt_PreservesAlreadyEncrypted(t *testing.T) {
	v := newTestVault(t)
	alreadyEnc := encPrefix + "dGVzdA=="
	graders := []store.GraderDef{
		{ID: "g1", Type: "llm_judge", Config: map[string]any{"api_key": alreadyEnc}},
	}
	enc, err := v.EncryptGraders(graders)
	if err != nil {
		t.Fatalf("EncryptGraders: %v", err)
	}
	// Should not double-encrypt
	if enc[0].Config["api_key"] != alreadyEnc {
		t.Errorf("already-encrypted value should be unchanged, got %q", enc[0].Config["api_key"])
	}
}

func TestGraderVault_Encrypt_PreservesSentinel(t *testing.T) {
	v := newTestVault(t)
	graders := []store.GraderDef{
		{ID: "g1", Type: "llm_judge", Config: map[string]any{"api_key": "***"}},
	}
	enc, err := v.EncryptGraders(graders)
	if err != nil {
		t.Fatalf("EncryptGraders: %v", err)
	}
	if enc[0].Config["api_key"] != "***" {
		t.Errorf("sentinel should be preserved, got %q", enc[0].Config["api_key"])
	}
}

func TestGraderVault_OriginalUnmodified(t *testing.T) {
	v := newTestVault(t)
	orig := []store.GraderDef{
		{ID: "g1", Type: "llm_judge", Config: map[string]any{"api_key": "sk-orig"}},
	}
	_, err := v.EncryptGraders(orig)
	if err != nil {
		t.Fatalf("EncryptGraders: %v", err)
	}
	// original slice should not be mutated
	if orig[0].Config["api_key"] != "sk-orig" {
		t.Error("EncryptGraders mutated original slice")
	}
}
