// Package crypto provides AES-256-GCM encryption and decryption via a KeyProvider interface.
package crypto

import (
	"fmt"
	"os"
)

// FileKeyProvider provides an AES-256 key from a file on disk.
type FileKeyProvider struct {
	path string
}

// NewFileKeyProvider creates a KeyProvider that reads a 32-byte key from the given file path.
func NewFileKeyProvider(path string) *FileKeyProvider {
	return &FileKeyProvider{path: path}
}

// Key reads the file and returns its contents as the key.
// The file must contain exactly 32 bytes.
func (p *FileKeyProvider) Key() ([]byte, error) {
	data, err := os.ReadFile(p.path)
	if err != nil {
		return nil, fmt.Errorf("crypto: read key file %q: %w", p.path, err)
	}
	if len(data) != 32 {
		return nil, fmt.Errorf("crypto: key file %q contains %d bytes, want 32", p.path, len(data))
	}
	return data, nil
}
