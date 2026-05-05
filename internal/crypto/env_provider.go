// Package crypto provides AES-256-GCM encryption and decryption via a KeyProvider interface.
package crypto

import (
	"encoding/base64"
	"fmt"
)

// EnvKeyProvider provides an AES-256 key from a base64-encoded string (typically from an env var).
type EnvKeyProvider struct {
	b64key string
}

// NewEnvKeyProvider creates a KeyProvider backed by the given base64-encoded key string.
func NewEnvKeyProvider(b64key string) *EnvKeyProvider {
	return &EnvKeyProvider{b64key: b64key}
}

// Key decodes the base64 key and validates it is exactly 32 bytes.
// Fallback order matches config.loadKeyFromEnv: StdEncoding → RawStdEncoding (WR-02).
func (p *EnvKeyProvider) Key() ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(p.b64key)
	if err != nil {
		// Also try RawStdEncoding (no padding) in case the operator omitted trailing '='.
		// This mirrors the fallback in config.loadKeyFromEnv so both layers accept the same inputs.
		key, err = base64.RawStdEncoding.DecodeString(p.b64key)
		if err != nil {
			return nil, fmt.Errorf("crypto: decode ENCRYPTION_KEY: not valid base64")
		}
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("crypto: ENCRYPTION_KEY decoded to %d bytes, want 32", len(key))
	}
	return key, nil
}
