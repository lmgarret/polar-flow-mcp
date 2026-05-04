// Package crypto provides AES-256-GCM encryption and decryption via a KeyProvider interface.
package crypto

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

// Encrypt encrypts plaintext with AES-256-GCM. Returns nonce(12)||ciphertext as a single []byte.
func (c *Cipher) Encrypt(plaintext []byte) ([]byte, error) {
	return nil, nil // stub
}

// Decrypt decrypts a blob of the form nonce(12)||ciphertext produced by Encrypt.
func (c *Cipher) Decrypt(blob []byte) ([]byte, error) {
	return nil, nil // stub
}
