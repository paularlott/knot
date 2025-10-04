package sshd

import (
	"strings"
	"testing"
)

func TestGenerateEd25519PrivateKey(t *testing.T) {
	key, err := GenerateEd25519PrivateKey()
	if err != nil {
		t.Fatalf("GenerateEd25519PrivateKey failed: %v", err)
	}

	if key == "" {
		t.Error("Generated key should not be empty")
	}

	if !strings.Contains(key, "BEGIN PRIVATE KEY") {
		t.Error("Key should contain PEM header")
	}

	if !strings.Contains(key, "END PRIVATE KEY") {
		t.Error("Key should contain PEM footer")
	}

	key2, err := GenerateEd25519PrivateKey()
	if err != nil {
		t.Fatalf("Second key generation failed: %v", err)
	}

	if key == key2 {
		t.Error("Generated keys should be unique")
	}
}
