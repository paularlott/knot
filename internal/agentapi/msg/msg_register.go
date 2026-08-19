package msg

import "github.com/paularlott/knot/internal/database/model"

// message sent from an agent to the server to register itself
type Register struct {
	SpaceId  string
	Version  string
	PeerPort uint16 // external port peers should dial for direct connections (0 = no direct)

	// Registration handshake: Nonce starts the challenge; Proof answers the
	// server's RegisterChallenge with HMAC(registrationKey, spaceId|nonce|serverNonce).
	// Both empty = pre-handshake agent.
	Nonce string
	Proof string

	// Log sink registration (0 = not a sink). When set, the space asks to
	// receive a mirror of the log records of the spaces owned by the same
	// user, delivered as CmdMirrorLog batches that the agent writes to this
	// local port in this format (vl | loki | gelf | json).
	LogSinkPort   int
	LogSinkFormat string
}

// message sent from the server to the agent in response to a register message
type RegisterResponse struct {
	Version                  string
	Success                  bool
	Error                    string // failure reason when Success is false
	SSHKeys                  []string
	SSHPrivateKey            string
	GitHubUsernames          []string
	Shell                    string
	SSHHostSigner            string
	WithTerminal             bool
	WithVSCodeTunnel         bool
	WithCodeServer           bool
	WithSSH                  bool
	WithRunCommand           bool
	Freeze                   bool
	AgentToken               string
	ServerURL                string
	AgentEndpoint            string
	HealthCheckType          string
	HealthCheckConfig        string
	HealthCheckSkipSSLVerify bool
	HealthCheckTimeout       uint32
	HealthCheckInterval      uint32
	HealthCheckMaxFailures   uint32
	HealthCheckAutoRestart   bool
	PortForwards             []model.PortForwardEntry
	DirectEnabled            bool   // if true, server supports direct agent-to-agent connections
	PeerSecret               string // zone-wide shared secret for direct peer auth
}
