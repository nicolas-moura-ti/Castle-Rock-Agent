package cluster

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"
)

// deriveKey takes a shared secret and returns a 32-byte key
// for use with AES-256. Using SHA256 is fast and prevents DoS
// attacks that would occur with Argon2id on every packet.
func deriveKey(secret string) []byte {
	hash := sha256.Sum256([]byte(secret))
	return hash[:]
}

// Encrypt payload using AES-GCM
// Payload structure: [nonce (12 bytes)][ciphertext]
func Encrypt(payload []byte, secret string) ([]byte, error) {
	key := deriveKey(secret)

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

	// Append nonce to the beginning of the ciphertext
	return append(nonce, ciphertext...), nil
}

// Decrypt payload using AES-GCM
func Decrypt(ciphertext []byte, secret string) ([]byte, error) {
	key := deriveKey(secret)

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
		return nil, errors.New("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]

	plaintext, err := aesgcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}
