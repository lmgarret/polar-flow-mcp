// Package crypto provides tests for the AES-256-GCM Cipher and KeyProvider implementations.
package crypto

import (
	"bytes"
	"encoding/base64"
	cryptorand "crypto/rand"
	"encoding/hex"
	"os"
	"testing"
)

// testKey returns a base64-encoded 32-byte zero key for testing.
func testKey() string {
	return base64.StdEncoding.EncodeToString(make([]byte, 32))
}

// testCipher returns a Cipher backed by the zero test key.
func testCipher(t *testing.T) *Cipher {
	t.Helper()
	p := NewEnvKeyProvider(testKey())
	return NewCipher(p)
}

// Test1: Encrypt produces a blob longer than 12 bytes.
func TestEncryptProducesNonEmptyBlob(t *testing.T) {
	c := testCipher(t)
	blob, err := c.Encrypt([]byte("hello"))
	if err != nil {
		t.Fatalf("Encrypt: unexpected error: %v", err)
	}
	if len(blob) <= 12 {
		t.Fatalf("Encrypt: blob length %d, want > 12", len(blob))
	}
}

// Test2: Decrypt(Encrypt(plaintext)) round-trips correctly.
func TestEncryptDecryptRoundTrip(t *testing.T) {
	c := testCipher(t)
	plaintext := make([]byte, 100)
	if _, err := cryptorand.Read(plaintext); err != nil {
		t.Fatalf("generate plaintext: %v", err)
	}
	blob, err := c.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	got, err := c.Decrypt(blob)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("Decrypt: plaintext mismatch")
	}
}

// Test3: Decrypt on a malformed blob (< 12 bytes) returns an error, not a panic.
func TestDecryptShortBlobReturnsError(t *testing.T) {
	c := testCipher(t)
	_, err := c.Decrypt([]byte("short"))
	if err == nil {
		t.Fatal("Decrypt: expected error for blob shorter than nonce, got nil")
	}
}

// Test4: Decrypt with wrong key returns error (GCM authentication failure).
func TestDecryptWrongKeyReturnsError(t *testing.T) {
	c := testCipher(t)
	blob, err := c.Encrypt([]byte("secret data"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Build a cipher with a different key (all 0x01 instead of all 0x00).
	wrongKey := make([]byte, 32)
	for i := range wrongKey {
		wrongKey[i] = 0x01
	}
	wrongB64 := base64.StdEncoding.EncodeToString(wrongKey)
	wrongCipher := NewCipher(NewEnvKeyProvider(wrongB64))

	_, err = wrongCipher.Decrypt(blob)
	if err == nil {
		t.Fatal("Decrypt: expected authentication failure with wrong key, got nil")
	}
}

// Test5 (D-06): 1000 Encrypt calls produce 1000 distinct nonces.
func TestNonceUniquenessAcross1000Encrypts(t *testing.T) {
	c := testCipher(t)
	nonces := make(map[string]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		blob, err := c.Encrypt([]byte("nonce-uniqueness-test"))
		if err != nil {
			t.Fatalf("Encrypt[%d]: %v", i, err)
		}
		if len(blob) < 12 {
			t.Fatalf("Encrypt[%d]: blob too short (%d bytes)", i, len(blob))
		}
		key := hex.EncodeToString(blob[:12])
		if _, dup := nonces[key]; dup {
			t.Fatalf("Encrypt[%d]: nonce collision detected: %s", i, key)
		}
		nonces[key] = struct{}{}
	}
	if len(nonces) != 1000 {
		t.Fatalf("expected 1000 distinct nonces, got %d", len(nonces))
	}
}

// Test6: EnvKeyProvider.Key() returns decoded bytes for a valid base64 32-byte key.
func TestEnvKeyProviderValidKey(t *testing.T) {
	rawKey := make([]byte, 32)
	for i := range rawKey {
		rawKey[i] = byte(i)
	}
	b64 := base64.StdEncoding.EncodeToString(rawKey)
	p := NewEnvKeyProvider(b64)
	got, err := p.Key()
	if err != nil {
		t.Fatalf("Key(): unexpected error: %v", err)
	}
	if !bytes.Equal(got, rawKey) {
		t.Fatal("Key(): returned bytes do not match original key")
	}
}

// Test7: EnvKeyProvider.Key() returns error if decoded key is not 32 bytes.
func TestEnvKeyProviderWrongLength(t *testing.T) {
	// Encode a 16-byte key — invalid for AES-256.
	shortKey := base64.StdEncoding.EncodeToString(make([]byte, 16))
	p := NewEnvKeyProvider(shortKey)
	_, err := p.Key()
	if err == nil {
		t.Fatal("Key(): expected error for 16-byte key, got nil")
	}
}

// Test8: FileKeyProvider.Key() returns 32 bytes from a tempfile with exactly 32 random bytes.
func TestFileKeyProviderValidFile(t *testing.T) {
	rawKey := make([]byte, 32)
	if _, err := cryptorand.Read(rawKey); err != nil {
		t.Fatalf("generate key: %v", err)
	}
	f, err := os.CreateTemp(t.TempDir(), "keyfile-*")
	if err != nil {
		t.Fatalf("create tempfile: %v", err)
	}
	if _, err := f.Write(rawKey); err != nil {
		t.Fatalf("write key: %v", err)
	}
	f.Close()

	p := NewFileKeyProvider(f.Name())
	got, err := p.Key()
	if err != nil {
		t.Fatalf("Key(): unexpected error: %v", err)
	}
	if !bytes.Equal(got, rawKey) {
		t.Fatal("Key(): returned bytes do not match file contents")
	}
}

// Test9: FileKeyProvider.Key() returns error if the file does not exist.
func TestFileKeyProviderMissingFile(t *testing.T) {
	p := NewFileKeyProvider("/nonexistent/path/to/key.bin")
	_, err := p.Key()
	if err == nil {
		t.Fatal("Key(): expected error for missing file, got nil")
	}
}
