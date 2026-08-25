package agent_client

import (
	"time"

	"github.com/paularlott/knot/internal/agentapi/msg"
)

// SendSSHAuthResult reports an SSH public-key authentication attempt to
// every knot server; each server's session handler decides whether to audit
// it (zone leaders only, gated by server.audit.space_sessions). Dropped
// rather than blocked when a server's queue is full — auth attempts are
// frequent enough that backpressure would stall the SSH handshake.
func (c *AgentClient) SendSSHAuthResult(success bool, fingerprint, remoteAddr string) {
	c.serverListMutex.RLock()
	for _, server := range c.serverList {
		select {
		case server.authChannel <- &msg.SSHAuthResultMessage{
			Success:        success,
			KeyFingerprint: fingerprint,
			RemoteAddr:     remoteAddr,
			When:           time.Now().UTC(),
		}:
		default:
			// Queue full, drop message
		}
	}
	c.serverListMutex.RUnlock()
}
