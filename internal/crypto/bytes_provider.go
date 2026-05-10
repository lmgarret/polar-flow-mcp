// Package crypto provides AES-256-GCM encryption and decryption via a KeyProvider interface.
package crypto

import "fmt"

// BytesKeyProvider provides an AES-256 key from a raw byte slice.
// Use this when the key has already been decoded and validated (e.g., config.EncryptionKey).
type BytesKeyProvider struct {
	key []byte
}

// NewBytesKeyProvider creates a KeyProvider backed by the given raw key bytes.
func NewBytesKeyProvider(key []byte) *BytesKeyProvider {
	return &BytesKeyProvider{key: key}
}

// Key returns the raw key, validating that it is exactly 32 bytes.
func (p *BytesKeyProvider) Key() ([]byte, error) {
	if len(p.key) != 32 {
		return nil, fmt.Errorf("crypto: BytesKeyProvider key must be 32 bytes, got %d", len(p.key))
	}
	return p.key, nil
}
