package crypt

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
	"time"
)

// Agent registration and agent-channel TLS are both derived from the server
// encryption key, which every member of a zone shares (enforced at startup):
// one key materialises the whole trust fabric with nothing to store or sync.
//
// The TLS certificate is identical on every server in the zone by
// construction — the key pair is derived, not generated — so a single
// fingerprint pinned into containers (and shown to manual-agent operators)
// authenticates whichever server an agent connects to.

const (
	agentRegistrationKeyLabel = "agent-registration-key"
	agentTLSCertLabel         = "agent-tls-cert-v1"
)

// AgentRegistrationKey derives the per-space secret an agent must prove
// possession of to register. Injected into the container at create time;
// shown alongside the space UUID for manual agents.
func AgentRegistrationKey(encryptionKey, spaceId string) string {
	return base64.RawURLEncoding.EncodeToString(derivedSecret(encryptionKey, agentRegistrationKeyLabel, spaceId))
}

// AgentRegistrationProof computes the registration proof: an HMAC over the
// space id and both handshake nonces, so a captured proof can't be replayed
// (each side contributes fresh randomness).
func AgentRegistrationProof(registrationKey, spaceId, agentNonce, serverNonce string) string {
	mac := hmac.New(sha256.New, []byte(registrationKey))
	fmt.Fprintf(mac, "%s|%s|%s", spaceId, agentNonce, serverNonce)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// VerifyAgentRegistrationProof reports whether a proof from an agent matches
// the expected value, in constant time.
func VerifyAgentRegistrationProof(expected, got string) bool {
	return hmac.Equal([]byte(expected), []byte(got))
}

// AgentTLSCert deterministically builds the zone's self-signed certificate
// for the agent listener. Held in memory only: regenerating at every boot
// yields the same certificate, so there is nothing to persist.
func AgentTLSCert(encryptionKey string) (tls.Certificate, error) {
	seed := derivedSecret(encryptionKey, agentTLSCertLabel, "")
	if len(seed) < ed25519.SeedSize {
		return tls.Certificate{}, fmt.Errorf("agent tls cert: short seed")
	}

	key := ed25519.NewKeyFromSeed(seed[:ed25519.SeedSize])

	serialBytes := derivedSecret(encryptionKey, agentTLSCertLabel+"-serial", "")[:16]
	serial := new(big.Int).SetBytes(serialBytes)
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "knot-agent-server",
			Organization: []string{"knot"},
		},
		NotBefore:             time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:              time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("agent tls cert: %w", err)
	}

	return tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  key,
	}, nil
}

// AgentTLSCertFingerprint returns the hex SHA-256 of the certificate's
// public key (SPKI) — the value pinned by agents. Pinning the key rather
// than the certificate bytes keeps the pin stable across minor encoding
// differences while still authenticating the server.
func AgentTLSCertFingerprint(encryptionKey string) (string, error) {
	cert, err := AgentTLSCert(encryptionKey)
	if err != nil {
		return "", err
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return "", err
	}
	spki, err := x509.MarshalPKIXPublicKey(leaf.PublicKey)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(spki)
	return hex.EncodeToString(sum[:]), nil
}

// NewAgentNonce returns fresh randomness for one side of the registration
// handshake.
func NewAgentNonce() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// derivedSecret returns the raw HMAC of label|scope under the encryption
// key; string secrets wrap it in base64url.
func derivedSecret(encryptionKey, label, scope string) []byte {
	mac := hmac.New(sha256.New, []byte(encryptionKey))
	fmt.Fprintf(mac, "%s|%s", label, scope)
	return mac.Sum(nil)
}
