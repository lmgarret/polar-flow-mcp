// Package config provides configuration loading and validation from environment variables.
// Load() is fail-closed: the server refuses to start unless all required fields are present
// and valid.
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"os"
)

// ProxySecretHeader is the HTTP header name that the reverse proxy must send carrying the
// shared secret. The auth middleware reads this header and compares it with PROXY_SHARED_SECRET
// using subtle.ConstantTimeCompare before reading any identity headers.
const ProxySecretHeader = "X-Proxy-Secret"

// Config holds all validated server configuration loaded from environment variables.
// Every field is populated (never zero-valued) after a successful Load().
type Config struct {
	// AuthProxy is the name of the reverse proxy in front of this server (e.g. "authelia",
	// "authentik", "oauth2-proxy"). Must not be "unconfigured" or empty.
	AuthProxy string

	// ProxySharedSecret is the secret shared between this server and the reverse proxy.
	// Sent by the proxy in the X-Proxy-Secret header and checked with constant-time compare.
	ProxySharedSecret string

	// IdentityHeader is the HTTP header injected by the reverse proxy that carries the
	// authenticated user identity. Defaults to "Remote-User".
	IdentityHeader string

	// BindAddress is the TCP address the HTTP server listens on. Defaults to "127.0.0.1".
	BindAddress string

	// KeyProviderType selects the encryption key source: "env" (read from ENCRYPTION_KEY)
	// or "file" (read from the file at ENCRYPTION_KEY_FILE). Defaults to "env".
	KeyProviderType string

	// EncryptionKey is the validated 32-byte AES-256 key loaded at startup.
	// Populated by Load() from ENCRYPTION_KEY (base64) or ENCRYPTION_KEY_FILE (raw bytes).
	EncryptionKey []byte
}

// Load reads configuration from environment variables, validates all required fields, and
// returns a fully populated *Config. It returns a descriptive, actionable error if any
// required field is missing or invalid. The server must call os.Exit(1) if Load() returns
// a non-nil error.
func Load() (*Config, error) {
	cfg := &Config{}

	if err := loadAuthFields(cfg); err != nil {
		return nil, err
	}

	loadOptionalFields(cfg)

	keyBytes, err := loadEncryptionKey(cfg.KeyProviderType)
	if err != nil {
		return nil, err
	}

	cfg.EncryptionKey = keyBytes

	return cfg, nil
}

// loadAuthFields validates and sets AUTH_PROXY and PROXY_SHARED_SECRET.
func loadAuthFields(cfg *Config) error {
	authProxy := os.Getenv("AUTH_PROXY")
	if authProxy == "" || authProxy == "unconfigured" {
		return errors.New(
			"AUTH_PROXY must be set to the name of your reverse proxy " +
				"(e.g. authelia, authentik, oauth2-proxy); " +
				"see https://github.com/lm/polar-flow-mcp/docs/deployment for configuration",
		)
	}
	cfg.AuthProxy = authProxy

	secret := os.Getenv("PROXY_SHARED_SECRET")
	if secret == "" {
		return errors.New(
			"PROXY_SHARED_SECRET must be set to a non-empty secret shared with your reverse proxy; " +
				"see deployment docs",
		)
	}
	cfg.ProxySharedSecret = secret

	return nil
}

// loadOptionalFields sets fields with defaults: IDENTITY_HEADER, BIND_ADDRESS, KEY_PROVIDER.
func loadOptionalFields(cfg *Config) {
	identityHeader := os.Getenv("IDENTITY_HEADER")
	if identityHeader == "" {
		identityHeader = "Remote-User"
	}
	cfg.IdentityHeader = identityHeader

	bindAddress := os.Getenv("BIND_ADDRESS")
	if bindAddress == "" {
		bindAddress = "127.0.0.1"
	}
	cfg.BindAddress = bindAddress

	keyProvider := os.Getenv("KEY_PROVIDER")
	if keyProvider == "" {
		keyProvider = "env"
	}
	cfg.KeyProviderType = keyProvider
}

// loadEncryptionKey loads and validates the 32-byte AES-256 key from the configured source.
func loadEncryptionKey(providerType string) ([]byte, error) {
	switch providerType {
	case "env":
		return loadKeyFromEnv()
	case "file":
		return loadKeyFromFile()
	default:
		return nil, fmt.Errorf("KEY_PROVIDER must be %q or %q; got %q", "env", "file", providerType)
	}
}

// loadKeyFromEnv decodes and validates ENCRYPTION_KEY (base64-encoded 32-byte key).
func loadKeyFromEnv() ([]byte, error) {
	keyB64 := os.Getenv("ENCRYPTION_KEY")
	if keyB64 == "" {
		return nil, errors.New(
			"ENCRYPTION_KEY must be set to a base64-encoded 32-byte key; " +
				"generate one with: openssl rand -base64 32",
		)
	}

	decoded, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		// Also try RawStdEncoding (no padding) in case the operator omitted trailing '='.
		decoded, err = base64.RawStdEncoding.DecodeString(keyB64)
		if err != nil {
			return nil, fmt.Errorf(
				"ENCRYPTION_KEY is not valid base64: %w; regenerate with: openssl rand -base64 32",
				err,
			)
		}
	}

	if len(decoded) != 32 {
		return nil, fmt.Errorf(
			"ENCRYPTION_KEY decoded length must be 32 bytes (got %d); "+
				"regenerate with: openssl rand -base64 32",
			len(decoded),
		)
	}

	return decoded, nil
}

// loadKeyFromFile reads and validates ENCRYPTION_KEY_FILE (raw 32-byte key file).
func loadKeyFromFile() ([]byte, error) {
	keyFile := os.Getenv("ENCRYPTION_KEY_FILE")
	if keyFile == "" {
		return nil, errors.New(
			"ENCRYPTION_KEY_FILE must be set to the path of a file containing a 32-byte key",
		)
	}

	data, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read ENCRYPTION_KEY_FILE %q: %w", keyFile, err)
	}

	if len(data) != 32 {
		return nil, fmt.Errorf(
			"ENCRYPTION_KEY_FILE %q must contain exactly 32 bytes (got %d)",
			keyFile,
			len(data),
		)
	}

	return data, nil
}

// LogStartupBanner emits a structured slog.Info banner that names the trusted identity header,
// the required secret header, and the declared auth proxy. Call after a successful Load().
func LogStartupBanner(cfg *Config) {
	slog.Info("polar-flow-mcp starting",
		"auth_proxy", cfg.AuthProxy,
		"identity_header", cfg.IdentityHeader,
		"secret_header", ProxySecretHeader,
		"bind_address", cfg.BindAddress,
	)
}
