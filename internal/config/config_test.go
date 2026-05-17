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
		"TRANSPORT",
		"DEV_USER_ID",
		"AUTH_PROXY",
		"PROXY_SHARED_SECRET",
		"IDENTITY_HEADER",
		"BIND_ADDRESS",
		"KEY_PROVIDER",
		"ENCRYPTION_KEY",
		"ENCRYPTION_KEY_FILE",
		"POLAR_CLIENT_ID",
		"POLAR_CLIENT_SECRET",
		"POLAR_REDIRECT_URL",
	} {
		t.Setenv(key, "")
	}
}

// setValidPolarEnv sets all three required Polar env vars to valid non-empty values.
// Call this in tests that verify non-Polar validation so they reach the desired failure point.
func setValidPolarEnv(t *testing.T) {
	t.Helper()
	t.Setenv("POLAR_CLIENT_ID", "test-client-id")
	t.Setenv("POLAR_CLIENT_SECRET", "test-client-secret")
	t.Setenv("POLAR_REDIRECT_URL", "https://example.com/oauth/callback")
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
	setValidPolarEnv(t)
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
	setValidPolarEnv(t)
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
	setValidPolarEnv(t)
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
	setValidPolarEnv(t)
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
	setValidPolarEnv(t)
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
	setValidPolarEnv(t)
	t.Setenv("ENCRYPTION_KEY", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.IdentityHeader != "Remote-User" {
		t.Errorf("expected IdentityHeader Remote-User; got %q", cfg.IdentityHeader)
	}
}

// TestLoadFailsIfPolarClientIDMissing verifies that Load() returns an error mentioning
// "POLAR_CLIENT_ID" when that env var is unset while all others are valid.
func TestLoadFailsIfPolarClientIDMissing(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("AUTH_PROXY", "authelia")
	t.Setenv("PROXY_SHARED_SECRET", "supersecret")
	t.Setenv("ENCRYPTION_KEY", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	// POLAR_CLIENT_ID left unset; set the other two Polar vars.
	t.Setenv("POLAR_CLIENT_SECRET", "test-client-secret")
	t.Setenv("POLAR_REDIRECT_URL", "https://example.com/oauth/callback")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for missing POLAR_CLIENT_ID, got nil")
	}
	if !strings.Contains(err.Error(), "POLAR_CLIENT_ID") {
		t.Errorf("error must mention POLAR_CLIENT_ID; got: %q", err.Error())
	}
}

// TestLoadFailsIfPolarClientSecretMissing verifies that Load() returns an error mentioning
// "POLAR_CLIENT_SECRET" when only POLAR_CLIENT_SECRET is unset.
func TestLoadFailsIfPolarClientSecretMissing(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("AUTH_PROXY", "authelia")
	t.Setenv("PROXY_SHARED_SECRET", "supersecret")
	t.Setenv("ENCRYPTION_KEY", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	t.Setenv("POLAR_CLIENT_ID", "cid")
	// POLAR_CLIENT_SECRET left unset.
	t.Setenv("POLAR_REDIRECT_URL", "https://example.com/oauth/callback")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for missing POLAR_CLIENT_SECRET, got nil")
	}
	if !strings.Contains(err.Error(), "POLAR_CLIENT_SECRET") {
		t.Errorf("error must mention POLAR_CLIENT_SECRET; got: %q", err.Error())
	}
}

// TestLoadFailsIfPolarRedirectURLMissing verifies that Load() returns an error mentioning
// "POLAR_REDIRECT_URL" when only POLAR_REDIRECT_URL is unset.
func TestLoadFailsIfPolarRedirectURLMissing(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("AUTH_PROXY", "authelia")
	t.Setenv("PROXY_SHARED_SECRET", "supersecret")
	t.Setenv("ENCRYPTION_KEY", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	t.Setenv("POLAR_CLIENT_ID", "cid")
	t.Setenv("POLAR_CLIENT_SECRET", "csec")
	// POLAR_REDIRECT_URL left unset.

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for missing POLAR_REDIRECT_URL, got nil")
	}
	if !strings.Contains(err.Error(), "POLAR_REDIRECT_URL") {
		t.Errorf("error must mention POLAR_REDIRECT_URL; got: %q", err.Error())
	}
}

// TestLoadSucceedsWithAllPolarFields verifies that Load() returns a *Config with all three
// Polar fields populated when all required env vars are set.
func TestLoadSucceedsWithAllPolarFields(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("AUTH_PROXY", "authelia")
	t.Setenv("PROXY_SHARED_SECRET", "supersecret")
	t.Setenv("ENCRYPTION_KEY", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	t.Setenv("POLAR_CLIENT_ID", "cid")
	t.Setenv("POLAR_CLIENT_SECRET", "csec")
	t.Setenv("POLAR_REDIRECT_URL", "https://example.com/oauth/callback")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected no error with all Polar fields set; got: %v", err)
	}
	if cfg.PolarClientID != "cid" {
		t.Errorf("PolarClientID: got %q, want %q", cfg.PolarClientID, "cid")
	}
	if cfg.PolarClientSecret != "csec" {
		t.Errorf("PolarClientSecret: got %q, want %q", cfg.PolarClientSecret, "csec")
	}
	if cfg.PolarRedirectURL != "https://example.com/oauth/callback" {
		t.Errorf("PolarRedirectURL: got %q, want %q", cfg.PolarRedirectURL, "https://example.com/oauth/callback")
	}
}

// validKey is a base64-encoded 32-byte zero key used across stdio tests.
const validKey = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="

// setValidStdioEnv sets all env vars needed for a complete TRANSPORT=stdio Load() call
// except TRANSPORT and DEV_USER_ID, which tests set explicitly.
func setValidStdioEnv(t *testing.T) {
	t.Helper()
	t.Setenv("ENCRYPTION_KEY", validKey)
	setValidPolarEnv(t)
}

// TestLoadStdioTransportSuccess verifies that TRANSPORT=stdio with DEV_USER_ID set
// succeeds and populates Transport and StdioUserID correctly, leaving auth fields empty.
func TestLoadStdioTransportSuccess(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("TRANSPORT", "stdio")
	t.Setenv("DEV_USER_ID", "polar-user-123")
	setValidStdioEnv(t)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected no error for TRANSPORT=stdio with DEV_USER_ID set; got: %v", err)
	}
	if cfg.Transport != "stdio" {
		t.Errorf("Transport: got %q, want %q", cfg.Transport, "stdio")
	}
	if cfg.StdioUserID != "polar-user-123" {
		t.Errorf("StdioUserID: got %q, want %q", cfg.StdioUserID, "polar-user-123")
	}
	if cfg.AuthProxy != "" {
		t.Errorf("AuthProxy must be empty in stdio mode; got %q", cfg.AuthProxy)
	}
	if cfg.ProxySharedSecret != "" {
		t.Errorf("ProxySharedSecret must be empty in stdio mode; got %q", cfg.ProxySharedSecret)
	}
}

// TestLoadStdioTransportMissingDevUserID verifies that TRANSPORT=stdio without DEV_USER_ID
// returns an error mentioning "DEV_USER_ID".
func TestLoadStdioTransportMissingDevUserID(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("TRANSPORT", "stdio")
	// DEV_USER_ID intentionally not set.
	setValidStdioEnv(t)

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for TRANSPORT=stdio with DEV_USER_ID unset, got nil")
	}
	if !strings.Contains(err.Error(), "DEV_USER_ID") {
		t.Errorf("error must mention DEV_USER_ID; got: %q", err.Error())
	}
}

// TestLoadHTTPTransportUnchanged verifies that TRANSPORT=http (explicit) behaves identically
// to the default (omitted TRANSPORT) and requires AUTH_PROXY and PROXY_SHARED_SECRET.
func TestLoadHTTPTransportUnchanged(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("TRANSPORT", "http")
	t.Setenv("AUTH_PROXY", "authelia")
	t.Setenv("PROXY_SHARED_SECRET", "supersecret")
	t.Setenv("ENCRYPTION_KEY", validKey)
	setValidPolarEnv(t)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected no error for TRANSPORT=http with all fields set; got: %v", err)
	}
	if cfg.Transport != "http" {
		t.Errorf("Transport: got %q, want %q", cfg.Transport, "http")
	}
	if cfg.AuthProxy != "authelia" {
		t.Errorf("AuthProxy: got %q, want %q", cfg.AuthProxy, "authelia")
	}
}

// TestLoadBadTransportValue verifies that an unrecognised TRANSPORT value returns an error
// mentioning "TRANSPORT".
func TestLoadBadTransportValue(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("TRANSPORT", "grpc")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for TRANSPORT=grpc, got nil")
	}
	if !strings.Contains(err.Error(), "TRANSPORT") {
		t.Errorf("error must mention TRANSPORT; got: %q", err.Error())
	}
}
