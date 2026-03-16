package cluster

import (
	"bytes"
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	secret := "my-super-secret-key"
	payload := []byte("hello world this is a test payload")

	encrypted, err := Encrypt(payload, secret)
	if err != nil {
		t.Fatalf("failed to encrypt: %v", err)
	}

	if bytes.Equal(encrypted, payload) {
		t.Fatalf("encrypted payload is identical to original")
	}

	decrypted, err := Decrypt(encrypted, secret)
	if err != nil {
		t.Fatalf("failed to decrypt: %v", err)
	}

	if !bytes.Equal(decrypted, payload) {
		t.Fatalf("decrypted payload does not match original. got %s, want %s", decrypted, payload)
	}
}

func TestDecryptInvalidLength(t *testing.T) {
	secret := "secret"

	// Too short to contain salt
	shortPayload := []byte("short")
	_, err := Decrypt(shortPayload, secret)
	if err == nil {
		t.Fatalf("expected error for ciphertext too short to contain salt, got nil")
	}

	// Contains salt but too short to contain nonce
	saltOnlyPayload := make([]byte, 16)
	_, err = Decrypt(saltOnlyPayload, secret)
	if err == nil {
		t.Fatalf("expected error for ciphertext too short to contain nonce, got nil")
	}
}

func TestDecryptInvalidSecret(t *testing.T) {
	secret1 := "secret1"
	secret2 := "secret2"
	payload := []byte("hello world")

	encrypted, err := Encrypt(payload, secret1)
	if err != nil {
		t.Fatalf("failed to encrypt: %v", err)
	}

	_, err = Decrypt(encrypted, secret2)
	if err == nil {
		t.Fatalf("expected error for wrong secret, got nil")
	}
}
