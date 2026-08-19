package container

import (
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/util/crypt"
)

// AgentRegistrationEnv returns the environment entries provisioning an
// agent's registration credentials: the per-space registration key it must
// prove possession of, and the fingerprint (sha256 of the certificate
// public key) pinning the zone's agent TLS certificate. Every server in the
// zone derives both from the same encryption key, so the values hold for
// whichever server the agent connects to.
func AgentRegistrationEnv(cfg *config.ServerConfig, spaceId string) []string {
	env := []string{
		config.CONFIG_ENV_PREFIX + "_REGISTRATION_KEY=" + crypt.AgentRegistrationKey(cfg.EncryptionKey, spaceId),
	}

	fingerprint, err := crypt.AgentTLSCertFingerprint(cfg.EncryptionKey)
	if err != nil {
		// The cert itself is regenerated at listener start and would fail
		// there too; keep going with just the key so registration still
		// works if that error is ever cleared.
		log.WithError(err).Error("failed to derive agent TLS certificate fingerprint")
		return env
	}
	return append(env, config.CONFIG_ENV_PREFIX+"_SERVER_CERT_FINGERPRINT="+fingerprint)
}
