package crypt

import (
	"crypto/rand"
	"encoding/base64"

	"github.com/google/uuid"
	"github.com/paularlott/knot/internal/log"
)

func GenerateAPIKey() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	id, err := uuid.NewV7()
	if err != nil {
		log.Fatal(err.Error())
	}
	b = append(b, id[:16]...)

	return base64.URLEncoding.EncodeToString(b), nil
}
