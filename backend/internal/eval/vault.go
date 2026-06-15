package eval

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/g8rswimmer/cogniflow/internal/crypto"
	"github.com/g8rswimmer/cogniflow/internal/store"
)

// encPrefix marks a grader config api_key value as AES-256-GCM ciphertext.
// Encrypted values are stored as "enc:<base64(ciphertext)>" in the DB.
const encPrefix = "enc:"

// minCiphertextLen is the minimum valid AES-256-GCM ciphertext length:
// 12-byte nonce + 1-byte plaintext minimum + 16-byte GCM tag.
const minCiphertextLen = 29

// sensitiveGraderTypes is the set of grader types that carry an api_key.
var sensitiveGraderTypes = map[string]bool{
	"llm_judge": true,
	"checklist": true,
}

// GraderVault encrypts and decrypts api_key fields in grader configs.
// It uses the same crypto.Cipher as ConfigVault but operates on GraderDef slices.
type GraderVault struct {
	cipher *crypto.Cipher
}

// NewGraderVault creates a GraderVault backed by the given Cipher.
func NewGraderVault(cipher *crypto.Cipher) *GraderVault {
	return &GraderVault{cipher: cipher}
}

// EncryptGraders returns a copy of the grader slice with api_key values encrypted.
// Values that are already encrypted ("enc:..."), empty, or the masked sentinel ("***")
// are left unchanged. Called before writing to the DB.
func (v *GraderVault) EncryptGraders(graders []store.GraderDef) ([]store.GraderDef, error) {
	result := make([]store.GraderDef, len(graders))
	for i, g := range graders {
		result[i] = g
		if !sensitiveGraderTypes[g.Type] || g.Config == nil {
			continue
		}
		raw, ok := g.Config["api_key"]
		if !ok {
			continue
		}
		strKey, ok := raw.(string)
		if !ok || strKey == "" || strKey == "***" {
			continue
		}
		// Treat as already-encrypted only when the prefix is present AND the payload
		// decodes to a byte slice large enough to be a valid AES-GCM ciphertext.
		// This prevents a real API key that starts with "enc:" from being skipped
		// and stored in plaintext.
		if strings.HasPrefix(strKey, encPrefix) {
			decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(strKey, encPrefix))
			if err == nil && len(decoded) >= minCiphertextLen {
				continue // genuinely already encrypted
			}
			// Falls through to encrypt: the "enc:" prefix was a coincidence.
		}
		ciphertext, err := v.cipher.Encrypt([]byte(strKey))
		if err != nil {
			return nil, fmt.Errorf("grader vault: encrypt grader %q: %w", g.ID, err)
		}
		cfg := cloneConfig(g.Config)
		cfg["api_key"] = encPrefix + base64.StdEncoding.EncodeToString(ciphertext)
		result[i].Config = cfg
	}
	return result, nil
}

// DecryptGraders returns a copy with api_key values decrypted.
// Only values prefixed with "enc:" are decrypted; others are returned as-is.
// Called after reading from the DB, before passing to the runner or evaluating graders.
func (v *GraderVault) DecryptGraders(graders []store.GraderDef) ([]store.GraderDef, error) {
	result := make([]store.GraderDef, len(graders))
	for i, g := range graders {
		result[i] = g
		if !sensitiveGraderTypes[g.Type] || g.Config == nil {
			continue
		}
		raw, ok := g.Config["api_key"]
		if !ok {
			continue
		}
		strKey, ok := raw.(string)
		if !ok || !strings.HasPrefix(strKey, encPrefix) {
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(strKey, encPrefix))
		if err != nil {
			return nil, fmt.Errorf("grader vault: base64 decode grader %q: %w", g.ID, err)
		}
		plaintext, err := v.cipher.Decrypt(decoded)
		if err != nil {
			return nil, fmt.Errorf("grader vault: decrypt grader %q: %w", g.ID, err)
		}
		cfg := cloneConfig(g.Config)
		cfg["api_key"] = string(plaintext)
		result[i].Config = cfg
	}
	return result, nil
}

// MaskGraders returns a copy with api_key values replaced by "***".
// Called before including grader configs in API responses.
func (v *GraderVault) MaskGraders(graders []store.GraderDef) []store.GraderDef {
	result := make([]store.GraderDef, len(graders))
	for i, g := range graders {
		result[i] = g
		if !sensitiveGraderTypes[g.Type] || g.Config == nil {
			continue
		}
		if _, ok := g.Config["api_key"]; !ok {
			continue
		}
		cfg := cloneConfig(g.Config)
		cfg["api_key"] = "***"
		result[i].Config = cfg
	}
	return result
}

// EncryptValue encrypts a plain-text string using the same AES-256-GCM cipher
// and "enc:<base64>" format as grader api_keys. Used for the webhook_secret.
// Already-encrypted values (valid "enc:..." prefix) are returned unchanged.
func (v *GraderVault) EncryptValue(plaintext string) (string, error) {
	if strings.HasPrefix(plaintext, encPrefix) {
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(plaintext, encPrefix))
		if err == nil && len(decoded) >= minCiphertextLen {
			return plaintext, nil
		}
	}
	ciphertext, err := v.cipher.Encrypt([]byte(plaintext))
	if err != nil {
		return "", fmt.Errorf("grader vault: encrypt value: %w", err)
	}
	return encPrefix + base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptValue decrypts an "enc:..." string produced by EncryptValue.
// Returns the input unchanged if it is not prefixed with "enc:".
func (v *GraderVault) DecryptValue(encrypted string) (string, error) {
	if !strings.HasPrefix(encrypted, encPrefix) {
		return encrypted, nil
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(encrypted, encPrefix))
	if err != nil {
		return "", fmt.Errorf("grader vault: base64 decode value: %w", err)
	}
	plaintext, err := v.cipher.Decrypt(decoded)
	if err != nil {
		return "", fmt.Errorf("grader vault: decrypt value: %w", err)
	}
	return string(plaintext), nil
}

func cloneConfig(src map[string]any) map[string]any {
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
