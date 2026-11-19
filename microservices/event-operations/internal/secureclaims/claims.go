// Package secureclaims protects the frozen HukID request payload at rest.
// The caller must provide a base64-encoded 32-byte key through
// HUKID_CLAIMS_ENCRYPTION_KEY. The key is never persisted in PostgreSQL.
package secureclaims

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"os"
)

const KeyVersion = "env-aes256-gcm-v1"

func Encrypt(plaintext []byte) ([]byte, []byte, string, error) {
	key, err := keyFromEnvironment()
	if err != nil {
		return nil, nil, "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, "", err
	}
	return gcm.Seal(nil, nonce, plaintext, nil), nonce, KeyVersion, nil
}

func Decrypt(ciphertext, nonce []byte, keyVersion string) ([]byte, error) {
	if keyVersion != KeyVersion {
		return nil, errors.New("unsupported claims key version")
	}
	key, err := keyFromEnvironment()
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, nonce, ciphertext, nil)
}

func keyFromEnvironment() ([]byte, error) {
	raw := os.Getenv("HUKID_CLAIMS_ENCRYPTION_KEY")
	if raw == "" {
		return nil, errors.New("HUKID_CLAIMS_ENCRYPTION_KEY is required")
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil || len(key) != 32 {
		return nil, errors.New("HUKID_CLAIMS_ENCRYPTION_KEY must be base64 encoded 32 bytes")
	}
	return key, nil
}
