package secrets

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestLoadOrCreateKey_PersistsAcrossCalls(t *testing.T) {
	path := filepath.Join(t.TempDir(), "seed.key")

	key1, err := LoadOrCreateKey(path)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	if len(key1) != KeySize {
		t.Fatalf("expected key length %d, got %d", KeySize, len(key1))
	}

	key2, err := LoadOrCreateKey(path)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if !bytes.Equal(key1, key2) {
		t.Fatal("expected same key to be reloaded from disk")
	}
}

func TestEncryptDecrypt_Roundtrip(t *testing.T) {
	key := make([]byte, KeySize)
	plaintext := []byte("super-secret-value")

	ciphertext, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if bytes.Equal(ciphertext, plaintext) {
		t.Fatal("ciphertext should not equal plaintext")
	}

	decrypted, err := Decrypt(key, ciphertext)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("expected %q, got %q", plaintext, decrypted)
	}
}
