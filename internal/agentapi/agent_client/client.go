package agent_client

import (
	"strings"
	"sync"
	"time"

	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/config"
)

const (
	agentStatePingInterval = 2 * time.Second
	maxConnectionAttempts  = 5   // Maximum number of connection attempts before giving up
	logChannelBufferSize   = 200 // Size of the log message channel buffer
)

type AgentClient struct {
	defaultServerAddress   string // Default server address to connect to
	spaceId                string // Space ID for the agent client
	agentToken             string // Agent authentication token (same across all servers in zone)
	serverURL              string // Server URL for API calls (any server in zone)
	credentialsMutex       sync.RWMutex
	serverListMutex        sync.RWMutex
	serverList             map[string]*agentServer
	firstRegistrationMutex sync.Mutex
	firstRegistration      bool
	keysMutex              sync.Mutex
	lastPublicSSHKeys      []string
	lastGitHubUsernames    []string
	sshPort                int
	usingInternalSSH       bool
	sshConfirmedLive       bool
	withTerminal           bool
	withVSCodeTunnel       bool
	withCodeServer         bool
	withSSH                bool
	withRunCommand         bool
	httpPortMap            map[string]string
	httpsPortMap           map[string]string
	tcpPortMap             map[string]string
	logChannel             chan *msg.LogMessage
	logTempBuffer          []*msg.LogMessage
}

func NewAgentClient(defaultServerAddress, spaceId string) *AgentClient {
	return &AgentClient{
		defaultServerAddress: defaultServerAddress,
		spaceId:              spaceId,
		serverList:           make(map[string]*agentServer),
		firstRegistration:    true,
		lastPublicSSHKeys:    []string{},
		lastGitHubUsernames:  []string{},
		usingInternalSSH:     false,
		sshConfirmedLive:     false,
		withTerminal:         false,
		withVSCodeTunnel:     false,
		withCodeServer:       false,
		withSSH:              false,
		withRunCommand:       false,
		httpPortMap:          make(map[string]string),
		httpsPortMap:         make(map[string]string),
		tcpPortMap:           make(map[string]string),
		logChannel:           make(chan *msg.LogMessage, logChannelBufferSize),
	}
}

func (c *AgentClient) ConnectAndServe() {
	cfg := config.GetAgentConfig()
	c.sshPort = cfg.Port.SSH

	// Build a map of available http ports
	ports := cfg.Port.HTTPPorts
	c.httpPortMap = make(map[string]string, len(ports))
	for _, port := range ports {
		var name string
		if strings.Contains(port, "=") {
			parts := strings.Split(port, "=")
			port = parts[0]
			name = parts[1]
		} else {
			name = port
		}

		c.httpPortMap[port] = name
	}

	// Build a map of available https ports
	ports = cfg.Port.HTTPSPorts
	c.httpsPortMap = make(map[string]string, len(ports))
	for _, port := range ports {
		var name string
		if strings.Contains(port, "=") {
			parts := strings.Split(port, "=")
			port = parts[0]
			name = parts[1]
		} else {
			name = port
		}

		c.httpsPortMap[port] = name
	}

	// Build a map of the available tcp ports
	ports = cfg.Port.TCPPorts
	c.tcpPortMap = make(map[string]string, len(ports))
	for _, port := range ports {
		var name string
		if strings.Contains(port, "=") {
			parts := strings.Split(port, "=")
			port = parts[0]
			name = parts[1]
		} else {
			name = port
		}

		c.tcpPortMap[port] = name
	}

	// Connect to the server that started the agent
	c.serverListMutex.Lock()
	connection := NewAgentServer(c.defaultServerAddress, c.spaceId, c)
	c.serverList[connection.address] = connection
	connection.ConnectAndServe()

	c.serverListMutex.Unlock()

	// Init log message transport
	go c.initLogMessages()

	// Start periodic status reporting
	go c.reportState()
}

func (c *AgentClient) Shutdown() {
	c.serverListMutex.Lock()
	for _, server := range c.serverList {
		server.Shutdown()
	}
	c.serverList = make(map[string]*agentServer) // Clear the server list
	c.serverListMutex.Unlock()
}

func (c *AgentClient) GetSpaceId() string {
	return c.spaceId
}

func (c *AgentClient) GetAgentToken() string {
	c.credentialsMutex.RLock()
	defer c.credentialsMutex.RUnlock()
	return c.agentToken
}

func (c *AgentClient) GetServerURL() string {
	c.credentialsMutex.RLock()
	defer c.credentialsMutex.RUnlock()
	return c.serverURL
}
