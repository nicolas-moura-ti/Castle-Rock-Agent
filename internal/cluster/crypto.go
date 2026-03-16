package cluster

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"

	"golang.org/x/crypto/pbkdf2"
)

const (
	saltSize   = 16
	iterations = 100000
	keySize    = 32
)

// deriveKey takes a shared secret and a salt, returning a 32-byte key
// for use with AES-256.
func deriveKey(secret string, salt []byte) []byte {
	return pbkdf2.Key([]byte(secret), salt, iterations, keySize, sha256.New)
}

// Encrypt payload using AES-GCM
// Payload structure: [salt (16 bytes)][nonce (12 bytes)][ciphertext]
func Encrypt(payload []byte, secret string) ([]byte, error) {
	salt := make([]byte, saltSize)
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

	// Prepend salt and nonce to the ciphertext
	result := make([]byte, 0, saltSize+len(nonce)+len(ciphertext))
	result = append(result, salt...)
	result = append(result, nonce...)
	result = append(result, ciphertext...)

	return result, nil
}

// Decrypt payload using AES-GCM
// Expects payload structure: [salt (16 bytes)][nonce (12 bytes)][ciphertext]
func Decrypt(ciphertext []byte, secret string) ([]byte, error) {
	if len(ciphertext) < saltSize {
		return nil, errors.New("ciphertext too short to contain salt")
	}

	salt, ciphertext := ciphertext[:saltSize], ciphertext[saltSize:]
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
		return nil, errors.New("ciphertext too short to contain nonce")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]

	plaintext, err := aesgcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}
