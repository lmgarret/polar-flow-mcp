package auth

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// keyFile is the on-disk representation of the EdDSA signing key. Only the
// 32-byte seed is stored; the public key is derived on load.
type keyFile struct {
	Algorithm string `json:"alg"`
	Seed      string `json:"seed"` // base64 (std) of the 32-byte ed25519 seed
}

// loadOrCreateKey returns the EdDSA private key at path, creating (and
// persisting, chmod 600) a fresh one if the file does not exist. The signing
// key never leaves the host; persisting it keeps issued tokens valid across
// restarts.
func loadOrCreateKey(path string) (ed25519.PrivateKey, error) {
	if path == "" {
		return nil, errors.New("no signing key path configured")
	}

	data, err := os.ReadFile(path) //nolint:gosec // operator-controlled path
	switch {
	case err == nil:
		return decodeKey(data)
	case errors.Is(err, os.ErrNotExist):
		return createKey(path)
	default:
		return nil, fmt.Errorf("read key file %q: %w", path, err)
	}
}

func decodeKey(data []byte) (ed25519.PrivateKey, error) {
	var kf keyFile
	if err := json.Unmarshal(data, &kf); err != nil {
		return nil, fmt.Errorf("parse key file: %w", err)
	}
	seed, err := base64.StdEncoding.DecodeString(kf.Seed)
	if err != nil {
		return nil, fmt.Errorf("decode key seed: %w", err)
	}
	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("key seed is %d bytes, want %d", len(seed), ed25519.SeedSize)
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

func createKey(path string) (ed25519.PrivateKey, error) {
	_, priv, err := ed25519.GenerateKey(nil) // crypto/rand
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}
	kf := keyFile{
		Algorithm: "EdDSA",
		Seed:      base64.StdEncoding.EncodeToString(priv.Seed()),
	}
	body, err := json.MarshalIndent(kf, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode key: %w", err)
	}

	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create key dir %q: %w", dir, err)
		}
	}
	// O_EXCL so a concurrent start cannot clobber a key another process wrote.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		// Lost a race: another process created it first — read theirs.
		if errors.Is(err, os.ErrExist) {
			data, rerr := os.ReadFile(path) //nolint:gosec // operator-controlled path
			if rerr != nil {
				return nil, fmt.Errorf("read key file after race %q: %w", path, rerr)
			}
			return decodeKey(data)
		}
		return nil, fmt.Errorf("create key file %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(body); err != nil {
		return nil, fmt.Errorf("write key file %q: %w", path, err)
	}
	return priv, nil
}

// keyID derives a stable short key id from the public key (for the JWT kid
// header), so a future key rotation is distinguishable in tokens.
func keyID(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return base64.RawURLEncoding.EncodeToString(sum[:8])
}
