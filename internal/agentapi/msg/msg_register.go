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
	SSHKey           string
	GitHubUsername   string
	Shell            string
	SSHHostSigner    string
	WithTerminal     bool
	WithVSCodeTunnel bool
	WithCodeServer   bool
	WithSSH          bool
}
