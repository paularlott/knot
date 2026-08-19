package crypt

import (
	"crypto/tls"
	"crypto/x509"
	"testing"
)

func TestAgentRegistrationKeyIsStableAndScoped(t *testing.T) {
	a := AgentRegistrationKey("key1", "space1")
	if a == "" {
		t.Fatal("empty key")
	}
	if a != AgentRegistrationKey("key1", "space1") {
		t.Fatal("key not deterministic")
	}
	if a == AgentRegistrationKey("key1", "space2") {
		t.Fatal("key not scoped to the space")
	}
	if a == AgentRegistrationKey("key2", "space1") {
		t.Fatal("key not scoped to the encryption key")
	}
}

func TestAgentRegistrationProofVerifies(t *testing.T) {
	key := AgentRegistrationKey("key1", "space1")
	proof := AgentRegistrationProof(key, "space1", "agent-nonce", "server-nonce")

	if !VerifyAgentRegistrationProof(proof, AgentRegistrationProof(key, "space1", "agent-nonce", "server-nonce")) {
		t.Fatal("valid proof rejected")
	}
	// Any altered input invalidates the proof.
	for _, got := range []string{
		AgentRegistrationProof(AgentRegistrationKey("key1", "space2"), "space1", "agent-nonce", "server-nonce"),
		AgentRegistrationProof(key, "space1", "other-agent-nonce", "server-nonce"),
		AgentRegistrationProof(key, "space1", "agent-nonce", "other-server-nonce"),
		"",
	} {
		if VerifyAgentRegistrationProof(proof, got) {
			t.Fatalf("invalid proof accepted: %q", got)
		}
	}
}

func TestAgentTLSCertDeterministicAcrossZone(t *testing.T) {
	c1, err := AgentTLSCert("shared-key")
	if err != nil {
		t.Fatal(err)
	}
	c2, err := AgentTLSCert("shared-key")
	if err != nil {
		t.Fatal(err)
	}
	if !certsEqual(t, c1, c2) {
		t.Fatal("same encryption key produced different certs — pin would break across the zone")
	}

	f1, err := AgentTLSCertFingerprint("shared-key")
	if err != nil {
		t.Fatal(err)
	}
	f2, err := AgentTLSCertFingerprint("shared-key")
	if err != nil {
		t.Fatal(err)
	}
	if f1 != f2 {
		t.Fatal("fingerprint not deterministic")
	}

	other, err := AgentTLSCertFingerprint("different-key")
	if err != nil {
		t.Fatal(err)
	}
	if other == f1 {
		t.Fatal("different encryption keys produced the same fingerprint")
	}
}

func certsEqual(t *testing.T, a, b tls.Certificate) bool {
	t.Helper()
	// The private keys are deterministic; compare through the parsed leaf's
	// public key and the raw certificate bytes.
	if len(a.Certificate) != len(b.Certificate) {
		return false
	}
	la, err := x509.ParseCertificate(a.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	lb, err := x509.ParseCertificate(b.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	return la.Equal(lb)
}

func TestNewAgentNonceUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		n, err := NewAgentNonce()
		if err != nil {
			t.Fatal(err)
		}
		if seen[n] {
			t.Fatal("duplicate nonce")
		}
		seen[n] = true
	}
}
