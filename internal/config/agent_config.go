package config

type AgentConfig struct {
	Endpoint             string
	SpaceID              string
	UpdateAuthorizedKeys bool
	ServicePassword      string
	VSCodeTunnel         string
	SyslogPort           int
	APIPort              int
	DisableTerminal      bool
	DisableSpaceIO       bool
	MethodsFile          string
	DNSResolver          bool
	Port                 PortConfig
	TLS                  TLSConfig

	// Registration handshake: the per-space key proving the agent may
	// register for SpaceID, and the pinned fingerprint (sha256 of the
	// server certificate's public key) authenticating the agent listener.
	// Injected into containers at create time; passed by hand for manual
	// agents. Either may be empty for pre-upgrade provisioning.
	RegistrationKey       string
	ServerCertFingerprint string
}

type PortConfig struct {
	CodeServer int
	VNCHttp    int
	SSH        int
	TCPPorts   []string
	HTTPPorts  []string
	HTTPSPorts []string
}

// Global configuration instance
var (
	agentConfig *AgentConfig
)

// SetAgentConfig sets the global agent configuration
func SetAgentConfig(config *AgentConfig) {
	agentConfig = config
}

// GetAgentConfig returns the global agent configuration
func GetAgentConfig() *AgentConfig {
	return agentConfig
}
