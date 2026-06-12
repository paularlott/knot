package util

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUpdateSSHPrivateKey(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	privateKey := "-----BEGIN OPENSSH PRIVATE KEY-----\ntest\n-----END OPENSSH PRIVATE KEY-----"
	if err := UpdateSSHPrivateKey(privateKey); err != nil {
		t.Fatalf("UpdateSSHPrivateKey() error = %v", err)
	}

	keyPath := filepath.Join(home, ".ssh", "id_ed25519")
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("expected private key to be written: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("private key mode = %o, want 0600", info.Mode().Perm())
	}

	got, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("failed reading private key: %v", err)
	}
	if string(got) != privateKey+"\n" {
		t.Fatalf("private key content = %q, want %q", string(got), privateKey+"\n")
	}

	if err := UpdateSSHPrivateKey(""); err != nil {
		t.Fatalf("UpdateSSHPrivateKey(\"\") error = %v", err)
	}
	if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
		t.Fatalf("expected private key to be removed, stat error = %v", err)
	}
}

func TestUpdateSSHPrivateKeyRSA(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	rsaKey := "-----BEGIN RSA PRIVATE KEY-----\ntest\n-----END RSA PRIVATE KEY-----"
	if err := UpdateSSHPrivateKey(rsaKey); err != nil {
		t.Fatalf("UpdateSSHPrivateKey() error = %v", err)
	}

	keyPath := filepath.Join(home, ".ssh", "id_rsa")
	if _, err := os.Stat(keyPath); err != nil {
		t.Fatalf("expected RSA private key at id_rsa: %v", err)
	}

	ed25519Path := filepath.Join(home, ".ssh", "id_ed25519")
	if _, err := os.Stat(ed25519Path); !os.IsNotExist(err) {
		t.Fatalf("expected id_ed25519 to not exist")
	}
}

func TestUpdateSSHPrivateKeyCleansOldKeyOnTypeChange(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	ed25519Key := "-----BEGIN OPENSSH PRIVATE KEY-----\ntest\n-----END OPENSSH PRIVATE KEY-----"
	if err := UpdateSSHPrivateKey(ed25519Key); err != nil {
		t.Fatalf("UpdateSSHPrivateKey() error = %v", err)
	}

	rsaKey := "-----BEGIN RSA PRIVATE KEY-----\ntest\n-----END RSA PRIVATE KEY-----"
	if err := UpdateSSHPrivateKey(rsaKey); err != nil {
		t.Fatalf("UpdateSSHPrivateKey() error = %v", err)
	}

	ed25519Path := filepath.Join(home, ".ssh", "id_ed25519")
	if _, err := os.Stat(ed25519Path); !os.IsNotExist(err) {
		t.Fatalf("expected old id_ed25519 to be removed after switching to RSA")
	}

	rsaPath := filepath.Join(home, ".ssh", "id_rsa")
	if _, err := os.Stat(rsaPath); err != nil {
		t.Fatalf("expected RSA key to exist: %v", err)
	}
}
