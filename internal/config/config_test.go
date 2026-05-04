// Package config_test tests fail-closed validation in config.Load().
package config_test

import (
	"os"
	"strings"
	"testing"

	"github.com/lm/polar-flow-mcp/internal/config"
)

// clearConfigEnv clears all environment variables read by config.Load().
func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"AUTH_PROXY",
		"PROXY_SHARED_SECRET",
		"IDENTITY_HEADER",
		"BIND_ADDRESS",
		"KEY_PROVIDER",
		"ENCRYPTION_KEY",
		"ENCRYPTION_KEY_FILE",
	} {
		t.Setenv(key, "")
	}
}

// TestLoadAuthProxyUnconfigured verifies that AUTH_PROXY=unconfigured returns an error
// containing "AUTH_PROXY" and a documentation pointer.
func TestLoadAuthProxyUnconfigured(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("AUTH_PROXY", "unconfigured")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for AUTH_PROXY=unconfigured, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "AUTH_PROXY") {
		t.Errorf("error must mention AUTH_PROXY; got: %q", msg)
	}
	if !strings.Contains(msg, "docs") {
		t.Errorf("error must contain docs pointer; got: %q", msg)
	}
}

// TestLoadAuthProxyEmpty verifies that AUTH_PROXY="" returns an error containing "AUTH_PROXY".
func TestLoadAuthProxyEmpty(t *testing.T) {
	clearConfigEnv(t)
	// AUTH_PROXY already cleared to "" by clearConfigEnv.

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for AUTH_PROXY empty, got nil")
	}
	if !strings.Contains(err.Error(), "AUTH_PROXY") {
		t.Errorf("error must mention AUTH_PROXY; got: %q", err.Error())
	}
}

// TestLoadProxySharedSecretEmpty verifies that a valid AUTH_PROXY but empty PROXY_SHARED_SECRET
// returns an error containing "PROXY_SHARED_SECRET".
func TestLoadProxySharedSecretEmpty(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("AUTH_PROXY", "authelia")
	// PROXY_SHARED_SECRET left empty.

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for empty PROXY_SHARED_SECRET, got nil")
	}
	if !strings.Contains(err.Error(), "PROXY_SHARED_SECRET") {
		t.Errorf("error must mention PROXY_SHARED_SECRET; got: %q", err.Error())
	}
}

// TestLoadEncryptionKeyMissing verifies that valid AUTH_PROXY + PROXY_SHARED_SECRET but no
// ENCRYPTION_KEY and no ENCRYPTION_KEY_FILE returns an error containing "ENCRYPTION_KEY"
// and the openssl generation hint.
func TestLoadEncryptionKeyMissing(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("AUTH_PROXY", "authelia")
	t.Setenv("PROXY_SHARED_SECRET", "supersecret")
	// No ENCRYPTION_KEY, no ENCRYPTION_KEY_FILE.

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for missing encryption key, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "ENCRYPTION_KEY") {
		t.Errorf("error must mention ENCRYPTION_KEY; got: %q", msg)
	}
	if !strings.Contains(msg, "openssl rand -base64 32") {
		t.Errorf("error must contain openssl hint; got: %q", msg)
	}
}

// TestLoadEncryptionKeyValidBase64Succeeds verifies that a valid base64-encoded 32-byte
// ENCRYPTION_KEY results in a successful Load().
func TestLoadEncryptionKeyValidBase64Succeeds(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("AUTH_PROXY", "authelia")
	t.Setenv("PROXY_SHARED_SECRET", "supersecret")
	// 32 zero bytes base64-encoded = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	t.Setenv("ENCRYPTION_KEY", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected no error for valid 32-byte key; got: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil Config")
	}
	if len(cfg.EncryptionKey) != 32 {
		t.Errorf("expected EncryptionKey length 32; got %d", len(cfg.EncryptionKey))
	}
}

// TestLoadEncryptionKeyWrongLength verifies that a base64-encoded value decoding to != 32 bytes
// returns an error.
func TestLoadEncryptionKeyWrongLength(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("AUTH_PROXY", "authelia")
	t.Setenv("PROXY_SHARED_SECRET", "supersecret")
	// 16 zero bytes base64 — only 16 bytes, not 32.
	t.Setenv("ENCRYPTION_KEY", "AAAAAAAAAAAAAAAAAAAAAA==")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for wrong-length key, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "32") {
		t.Errorf("error must mention 32; got: %q", msg)
	}
}

// TestLoadEncryptionKeyFile verifies that ENCRYPTION_KEY_FILE pointing to a file
// containing 32 bytes results in a successful Load().
func TestLoadEncryptionKeyFile(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("AUTH_PROXY", "authelia")
	t.Setenv("PROXY_SHARED_SECRET", "supersecret")
	t.Setenv("KEY_PROVIDER", "file")

	// Write 32 zero bytes to a temp file.
	f, err := os.CreateTemp(t.TempDir(), "keyfile")
	if err != nil {
		t.Fatalf("failed to create temp key file: %v", err)
	}
	key := make([]byte, 32)
	if _, err := f.Write(key); err != nil {
		t.Fatalf("failed to write key to temp file: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("failed to close temp key file: %v", err)
	}
	t.Setenv("ENCRYPTION_KEY_FILE", f.Name())

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected no error for valid key file; got: %v", err)
	}
	if len(cfg.EncryptionKey) != 32 {
		t.Errorf("expected EncryptionKey length 32; got %d", len(cfg.EncryptionKey))
	}
}

// TestLoadBindAddressDefault verifies that BIND_ADDRESS defaults to "127.0.0.1".
func TestLoadBindAddressDefault(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("AUTH_PROXY", "authelia")
	t.Setenv("PROXY_SHARED_SECRET", "supersecret")
	t.Setenv("ENCRYPTION_KEY", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.BindAddress != "127.0.0.1" {
		t.Errorf("expected BindAddress 127.0.0.1; got %q", cfg.BindAddress)
	}
}

// TestLoadIdentityHeaderDefault verifies that IDENTITY_HEADER defaults to "Remote-User".
func TestLoadIdentityHeaderDefault(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("AUTH_PROXY", "authelia")
	t.Setenv("PROXY_SHARED_SECRET", "supersecret")
	t.Setenv("ENCRYPTION_KEY", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.IdentityHeader != "Remote-User" {
		t.Errorf("expected IdentityHeader Remote-User; got %q", cfg.IdentityHeader)
	}
}
