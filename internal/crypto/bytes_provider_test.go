package crypto

import (
	"testing"
)

func TestBytesKeyProvider_ReturnsKey(t *testing.T) {
	key := make([]byte, 32)
	p := NewBytesKeyProvider(key)
	got, err := p.Key()
	if err != nil {
		t.Fatalf("Key() returned error: %v", err)
	}
	if len(got) != 32 {
		t.Errorf("Key() returned %d bytes, want 32", len(got))
	}
	// Same slice returned
	if &got[0] != &key[0] {
		t.Error("Key() did not return the same slice")
	}
}

func TestBytesKeyProvider_RejectsWrongLength(t *testing.T) {
	p := NewBytesKeyProvider(make([]byte, 16))
	got, err := p.Key()
	if err == nil {
		t.Fatal("Key() should return error for 16-byte key, got nil")
	}
	if got != nil {
		t.Errorf("Key() should return nil on error, got %v", got)
	}
	// Error should mention "32"
	if err.Error() == "" {
		t.Error("error message should be non-empty")
	}
}
