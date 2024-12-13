package sshd

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
)

func GenerateEd25519PrivateKey() (string, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		fmt.Printf("Generation error : %s", err)
		return "", err
	}

	b, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return "", err
	}

	block := &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: b,
	}

	privateKeyPEM := pem.EncodeToMemory(block)

	return string(privateKeyPEM), nil
}
