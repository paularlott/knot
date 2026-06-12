package crypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"

	"github.com/paularlott/knot/internal/log"
)

func CreateKey() string {
	chars := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	bytes := make([]byte, 32)
	_, err := rand.Read(bytes)
	if err != nil {
		log.Fatal(err.Error())
	}

	for i, b := range bytes {
		bytes[i] = chars[b%byte(len(chars))]
	}

	return string(bytes)
}

func Encrypt(key string, text string) string {
	aes, err := aes.NewCipher([]byte(key))
	if err != nil {
		log.Fatal(err.Error())
	}

	gcm, err := cipher.NewGCM(aes)
	if err != nil {
		log.Fatal(err.Error())
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		log.Fatal(err.Error())
	}

	return string(gcm.Seal(nonce, nonce, []byte(text), nil))
}

func EncryptB64(key string, text string) string {
	encrypted := Encrypt(key, text)
	return base64.StdEncoding.EncodeToString([]byte(encrypted))
}

func Decrypt(key string, text string) string {
	aes, err := aes.NewCipher([]byte(key))
	if err != nil {
		log.Fatal(err.Error())
	}

	gcm, err := cipher.NewGCM(aes)
	if err != nil {
		log.Fatal(err.Error())
	}

	nonceSize := gcm.NonceSize()
	nonce, ciphertext := []byte(text)[:nonceSize], []byte(text)[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		log.Fatal(err.Error())
	}

	return string(plaintext)
}

func DecryptB64(key string, text string) string {
	decoded, err := base64.StdEncoding.DecodeString(text)
	if err != nil {
		log.Fatal(err.Error())
	}

	if len(decoded) < 16 {
		return ""
	}

	return Decrypt(key, string(decoded))
}

func EncryptB64Safe(key string, text string) string {
	if text == "" {
		return ""
	}

	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return ""
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return ""
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return ""
	}

	return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(text), nil))
}

func DecryptB64Safe(key string, text string) string {
	if text == "" {
		return ""
	}
	decoded, err := base64.StdEncoding.DecodeString(text)
	if err != nil {
		return text
	}
	if len(decoded) < 16 {
		return ""
	}
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return text
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return text
	}
	nonceSize := gcm.NonceSize()
	if len(decoded) < nonceSize {
		return text
	}
	nonce, ciphertext := decoded[:nonceSize], decoded[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return text
	}
	return string(plaintext)
}
