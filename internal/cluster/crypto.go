package cluster

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"

	"golang.org/x/crypto/argon2"
)

// deriveKey takes a shared secret and a salt and returns a 32-byte key
// for use with AES-256. Uses Argon2id for strong GPU resistance.
func deriveKey(secret string, salt []byte) []byte {
	return argon2.IDKey([]byte(secret), salt, 1, 64*1024, 4, 32)
}

// Encrypt payload using AES-GCM
// Payload structure: [salt (16 bytes)][nonce (12 bytes)][ciphertext]
func Encrypt(payload []byte, secret string) ([]byte, error) {
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}

	key := deriveKey(secret, salt)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, aesgcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := aesgcm.Seal(nil, nonce, payload, nil)

	// Append salt + nonce to the beginning of the ciphertext
	out := append(salt, nonce...)
	out = append(out, ciphertext...)

	return out, nil
}

// Decrypt payload using AES-GCM
func Decrypt(ciphertext []byte, secret string) ([]byte, error) {
	// Need at least 16 (salt) + 12 (nonce) bytes
	if len(ciphertext) < 16+12 {
		return nil, errors.New("ciphertext too short")
	}

	salt, ciphertext := ciphertext[:16], ciphertext[16:]
	key := deriveKey(secret, salt)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := aesgcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("ciphertext missing nonce")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]

	plaintext, err := aesgcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}
