package cluster

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"

	"golang.org/x/crypto/argon2"
)

const (
	saltSize   = 16
	argonTime  = 3
	argonMem   = 64 * 1024
	argonThred = 4
	keySize    = 32
)

// deriveKey takes a shared secret and a salt, returning a 32-byte key
// for use with AES-256. Uses Argon2id for strong GPU resistance.
func deriveKey(secret string, salt []byte) []byte {
	return argon2.IDKey([]byte(secret), salt, argonTime, argonMem, argonThred, keySize)
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
