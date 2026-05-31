package crypto

import (
	"bytes"
	"testing"
)

func validKey() []byte {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	return key
}

func TestNewCipher_ValidKey(t *testing.T) {
	c, err := NewCipher(validKey())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil cipher")
	}
}

func TestNewCipher_ShortKey(t *testing.T) {
	_, err := NewCipher(make([]byte, 16))
	if err == nil {
		t.Fatal("expected error for 16-byte key")
	}
}

func TestNewCipher_LongKey(t *testing.T) {
	_, err := NewCipher(make([]byte, 64))
	if err == nil {
		t.Fatal("expected error for 64-byte key")
	}
}

func TestNewCipher_EmptyKey(t *testing.T) {
	_, err := NewCipher([]byte{})
	if err == nil {
		t.Fatal("expected error for empty key")
	}
}

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	c, _ := NewCipher(validKey())
	plaintext := []byte("hello, cogniflow!")

	ciphertext, err := c.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if bytes.Equal(ciphertext, plaintext) {
		t.Fatal("ciphertext should differ from plaintext")
	}

	got, err := c.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("expected %q, got %q", plaintext, got)
	}
}

func TestEncryptDecrypt_EmptyPayload(t *testing.T) {
	c, _ := NewCipher(validKey())
	plaintext := []byte("")

	ct, err := c.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypt empty: %v", err)
	}
	pt, err := c.Decrypt(ct)
	if err != nil {
		t.Fatalf("decrypt empty: %v", err)
	}
	if !bytes.Equal(pt, plaintext) {
		t.Fatalf("expected empty plaintext, got %q", pt)
	}
}

func TestEncryptDecrypt_LargePayload(t *testing.T) {
	c, _ := NewCipher(validKey())
	plaintext := bytes.Repeat([]byte("secret key value!"), 1000)

	ct, err := c.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypt large: %v", err)
	}
	pt, err := c.Decrypt(ct)
	if err != nil {
		t.Fatalf("decrypt large: %v", err)
	}
	if !bytes.Equal(pt, plaintext) {
		t.Fatal("large payload round-trip mismatch")
	}
}

func TestEncrypt_UniqueNonceEachCall(t *testing.T) {
	c, _ := NewCipher(validKey())
	plaintext := []byte("same plaintext")

	ct1, _ := c.Encrypt(plaintext)
	ct2, _ := c.Encrypt(plaintext)

	// Different nonces → different ciphertexts.
	if bytes.Equal(ct1, ct2) {
		t.Fatal("two encryptions of the same plaintext should produce different ciphertexts")
	}
}

func TestDecrypt_TamperedData(t *testing.T) {
	c, _ := NewCipher(validKey())
	ct, _ := c.Encrypt([]byte("real secret"))

	// Flip a bit in the ciphertext body.
	ct[len(ct)-1] ^= 0xff

	_, err := c.Decrypt(ct)
	if err == nil {
		t.Fatal("expected authentication error on tampered ciphertext")
	}
}

func TestDecrypt_TooShort(t *testing.T) {
	c, _ := NewCipher(validKey())
	_, err := c.Decrypt([]byte("short"))
	if err == nil {
		t.Fatal("expected error for ciphertext shorter than nonce")
	}
}

func TestDecrypt_WrongKey(t *testing.T) {
	c1, _ := NewCipher(validKey())
	key2 := validKey()
	key2[0] ^= 0xff
	c2, _ := NewCipher(key2)

	ct, _ := c1.Encrypt([]byte("secret"))
	_, err := c2.Decrypt(ct)
	if err == nil {
		t.Fatal("expected error decrypting with wrong key")
	}
}

func TestNewCipher_KeyIsCopied(t *testing.T) {
	key := validKey()
	c, _ := NewCipher(key)
	// Modify original key after construction.
	key[0] = 0xff

	plaintext := []byte("test")
	ct, _ := c.Encrypt(plaintext)
	pt, err := c.Decrypt(ct)
	if err != nil {
		t.Fatalf("cipher should use copied key: %v", err)
	}
	if !bytes.Equal(pt, plaintext) {
		t.Fatal("round-trip failed after mutating original key")
	}
}
