//go:build integration

package suites

import (
	"crypto/tls"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/paularlott/knot/integration-tests/harness"
	"github.com/paularlott/knot/internal/agentapi/msg"
)

// registerAttempt opens a TLS connection to the agent listener and sends a
// registration for the space, impersonating an attacker that reaches the
// listener without holding the space's registration key.
func registerAttempt(t *testing.T, agentPort int, spaceId, nonce, proof string) *msg.RegisterResponse {
	t.Helper()
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp",
		net.JoinHostPort("127.0.0.1", strconv.Itoa(agentPort)),
		&tls.Config{InsecureSkipVerify: true}) // no pin: an attacker has none
	if err != nil {
		t.Fatalf("dial agent listener: %v", err)
	}
	defer conn.Close()

	if err := msg.WriteMessage(conn, &msg.Register{
		SpaceId: spaceId,
		Version: "0.0.0",
		Nonce:   nonce,
		Proof:   proof,
	}); err != nil {
		t.Fatalf("write register: %v", err)
	}

	if nonce != "" {
		// Answer the challenge like a real agent would — with a bogus proof.
		var challenge msg.RegisterChallenge
		if err := msg.ReadMessage(conn, &challenge); err != nil {
			t.Fatalf("read challenge: %v", err)
		}
		if err := msg.WriteMessage(conn, &msg.Register{SpaceId: spaceId, Proof: proof}); err != nil {
			t.Fatalf("write proof: %v", err)
		}
	}

	var response msg.RegisterResponse
	if err := msg.ReadMessage(conn, &response); err != nil {
		t.Fatalf("read response: %v", err)
	}
	return &response
}

// TestAgentRegistrationRequiresProof drives the original vulnerability's
// attack: a peer that reaches the agent listener and claims a space it has
// no key for. Both with no handshake and with a forged proof, registration
// must fail and no secrets (SSH private key, agent token) may appear in the
// response.
func TestAgentRegistrationRequiresProof(t *testing.T) {
	harness.Feature(t, "agent-auth")
	id := spaceFixture(t, "it-agauth", user1.Id, user1.Client)

	response := registerAttempt(t, server.AgentPort, id, "", "")
	if response.Success {
		t.Fatal("key-less registration accepted for a provisioned space")
	}
	if response.AgentToken != "" || response.SSHPrivateKey != "" {
		t.Fatal("secrets disclosed on rejected registration")
	}

	response = registerAttempt(t, server.AgentPort, id, "attacker-nonce", "forged-proof")
	if response.Success {
		t.Fatal("registration accepted with a forged proof")
	}
	if response.AgentToken != "" || response.SSHPrivateKey != "" {
		t.Fatal("secrets disclosed on rejected registration")
	}

	// The live session for the space must survive the attack untouched.
	if _, err := harness.TryRunCommand(user1.Client, id, 15, "echo", "still-alive"); err != nil {
		t.Fatalf("agent session disturbed by failed registrations: %v", err)
	}
}
