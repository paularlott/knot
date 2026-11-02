package crypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
)

// newGCM returns AES-GCM keyed with the server's encryption key.
func newGCM(encryptionKey string) (cipher.AEAD, error) {
	block, err := aes.NewCipher([]byte(encryptionKey))
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// randomB64URL returns n random bytes, base64url encoded.
func randomB64URL(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
