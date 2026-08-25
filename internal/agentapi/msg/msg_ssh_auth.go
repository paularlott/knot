package msg

import (
	"net"
	"time"

	"github.com/paularlott/knot/internal/log"
)

// SSHAuthResultMessage reports a public-key authentication attempt against
// the agent's SSH server: the outcome, the key's SHA256 fingerprint and the
// client address. Sent agent to server; the server records it under
// server.audit.space_sessions.
type SSHAuthResultMessage struct {
	Success        bool
	KeyFingerprint string
	RemoteAddr     string
	When           time.Time
}

func SendSSHAuthResult(conn net.Conn, result *SSHAuthResultMessage) error {
	logger := log.WithGroup("agent")
	err := WriteCommand(conn, CmdSSHPortAuthResult)
	if err != nil {
		logger.WithError(err).Error("writing ssh auth result command")
		return err
	}

	err = WriteMessage(conn, result)
	if err != nil {
		logger.WithError(err).Error("writing ssh auth result message")
		return err
	}

	return nil
}
