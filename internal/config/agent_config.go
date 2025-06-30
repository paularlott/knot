package config

type AgentConfig struct {
	Endpoint             string
	SpaceID              string
	UpdateAuthorizedKeys bool
	ServicePassword      string
	VSCodeTunnel         string
	AdvertiseAddr        string // TODO Remove this?
	SyslogPort           int
	APIPort              int

	// Port configuration
	Port PortConfig

	// TLS configuration
	TLS TLSConfig
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
