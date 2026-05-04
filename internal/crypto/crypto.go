// Package crypto provides AES-256-GCM encryption and decryption via a KeyProvider interface.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	cryptorand "crypto/rand"
	"errors"
	"fmt"
	"io"
)

const nonceSize = 12 // AES-256-GCM standard nonce size (bytes).

// KeyProvider is the interface for loading the AES-256-GCM encryption key.
// Phase 1 ships two implementations: env (reads ENCRYPTION_KEY) and file (reads ENCRYPTION_KEY_FILE).
type KeyProvider interface {
	// Key returns the 32-byte AES-256 key. Implementations may cache or re-read on each call.
	Key() ([]byte, error)
}

// Cipher wraps a KeyProvider and provides AES-256-GCM encrypt/decrypt.
type Cipher struct {
	provider KeyProvider
}

// NewCipher creates a new Cipher backed by the given KeyProvider.
func NewCipher(p KeyProvider) *Cipher {
	return &Cipher{provider: p}
}

// Encrypt encrypts plaintext with AES-256-GCM.
// Returns a blob of the form: nonce (12 bytes) || GCM ciphertext.
// A unique crypto/rand nonce is generated for every call.
func (c *Cipher) Encrypt(plaintext []byte) ([]byte, error) {
	key, err := c.provider.Key()
	if err != nil {
		return nil, fmt.Errorf("crypto: load key: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: create cipher block: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: create GCM: %w", err)
	}
	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(cryptorand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("crypto: generate nonce: %w", err)
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)
	// Return nonce || ciphertext as a single blob — never split.
	return append(nonce, ciphertext...), nil
}

// Decrypt decrypts a blob produced by Encrypt.
// Input must be at least nonceSize bytes; the first 12 bytes are the nonce.
func (c *Cipher) Decrypt(blob []byte) ([]byte, error) {
	if len(blob) < nonceSize {
		return nil, errors.New("crypto: blob too short to contain nonce")
	}
	key, err := c.provider.Key()
	if err != nil {
		return nil, fmt.Errorf("crypto: load key: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: create cipher block: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: create GCM: %w", err)
	}
	nonce := blob[:nonceSize]
	ciphertext := blob[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("crypto: decrypt: %w", err)
	}
	return plaintext, nil
}
