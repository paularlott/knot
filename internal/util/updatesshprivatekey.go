package util

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/paularlott/knot/internal/log"
)

func sshPrivateKeyFilename(privateKey string) string {
	if strings.Contains(privateKey, "BEGIN OPENSSH PRIVATE KEY") {
		return "id_ed25519"
	}
	if strings.Contains(privateKey, "BEGIN RSA PRIVATE KEY") {
		return "id_rsa"
	}
	if strings.Contains(privateKey, "BEGIN EC PRIVATE KEY") {
		return "id_ecdsa"
	}
	return "id_ed25519"
}

func UpdateSSHPrivateKey(privateKey string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	sshDir := filepath.Join(home, ".ssh")
	privateKey = strings.TrimSpace(privateKey)

	if privateKey == "" {
		log.Debug("Removing SSH private key")
		for _, name := range []string{"id_ed25519", "id_rsa", "id_ecdsa"} {
			keyPath := filepath.Join(sshDir, name)
			os.Remove(keyPath)
		}
		return nil
	}

	filename := sshPrivateKeyFilename(privateKey)
	keyPath := filepath.Join(sshDir, filename)

	log.Debug("Updating SSH private key", "path", keyPath)
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return err
	}
	if err := os.Chmod(sshDir, 0700); err != nil {
		return err
	}

	if err := os.WriteFile(keyPath, []byte(privateKey+"\n"), 0600); err != nil {
		return err
	}

	for _, name := range []string{"id_ed25519", "id_rsa", "id_ecdsa"} {
		if name == filename {
			continue
		}
		oldPath := filepath.Join(sshDir, name)
		os.Remove(oldPath)
	}

	return nil
}
