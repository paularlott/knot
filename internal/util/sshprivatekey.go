package util

import (
	"strings"

	"github.com/paularlott/knot/internal/util/crypt"
	gossh "golang.org/x/crypto/ssh"
)

func DecryptSSHPrivateKey(encryptionKey string, privateKey string) string {
	privateKey = crypt.DecryptB64Safe(encryptionKey, privateKey)
	if !validSSHPrivateKey(privateKey) {
		return ""
	}

	return strings.TrimSpace(privateKey)
}

func validSSHPrivateKey(privateKey string) bool {
	privateKey = strings.TrimSpace(privateKey)
	if privateKey == "" {
		return true
	}

	_, err := gossh.ParseRawPrivateKey([]byte(privateKey))
	return err == nil
}
