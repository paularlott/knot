package msg

// message sent from an agent to the server to register itself
type Register struct {
	SpaceId string
	Version string
}

// message sent from the server to the agent in response to a register message
type RegisterResponse struct {
	Version          string
	Success          bool
	SSHKeys          []string
	GitHubUsernames  []string
	Shell            string
	SSHHostSigner    string
	WithTerminal     bool
	WithVSCodeTunnel bool
	WithCodeServer   bool
	WithSSH          bool
	WithRunCommand   bool
	Freeze           bool
	AgentToken       string // Deterministic authentication token for the agent
	ServerURL        string // Public URL of the server for API calls
}
