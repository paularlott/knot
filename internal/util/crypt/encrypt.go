package crypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"

	"github.com/rs/zerolog/log"
)

func CreateKey() string {
  chars := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

  bytes := make([]byte, 32)
  _, err := rand.Read(bytes)
  if err != nil {
    log.Fatal().Msg(err.Error())
  }

  for i, b := range bytes {
    bytes[i] = chars[b % byte(len(chars))]
  }

  return string(bytes)
}

func Encrypt(key string, text string) string {
  aes, err := aes.NewCipher([]byte(key))
  if err != nil {
    log.Fatal().Msg(err.Error())
  }

  gcm, err := cipher.NewGCM(aes)
  if err != nil {
    log.Fatal().Msg(err.Error())
  }

  nonce := make([]byte, gcm.NonceSize())
  if _, err = rand.Read(nonce); err != nil {
    log.Fatal().Msg(err.Error())
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
    log.Fatal().Msg(err.Error())
  }

  gcm, err := cipher.NewGCM(aes)
  if err != nil {
    log.Fatal().Msg(err.Error())
  }

  nonceSize := gcm.NonceSize()
  nonce, ciphertext := []byte(text)[:nonceSize], []byte(text)[nonceSize:]
  plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
  if err != nil {
    log.Fatal().Msg(err.Error())
  }

  return string(plaintext)
}

func DecryptB64(key string, text string) string {
  decoded, err := base64.StdEncoding.DecodeString(text)
  if err != nil {
    log.Fatal().Msg(err.Error())
  }

  if len(decoded) < 16 {
    return ""
  }

  return Decrypt(key, string(decoded))
}
