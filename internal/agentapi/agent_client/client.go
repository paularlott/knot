package agent_client

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/methods"
)

const (
	agentStatePingInterval = 2 * time.Second
	maxConnectionAttempts  = 5   // Maximum number of connection attempts before giving up
	logChannelBufferSize   = 200 // Size of the log message channel buffer

	// rediscoverCooldown is how long a server that has been given up on is left
	// alone before the discovery loop is allowed to dial it again. Without it a
	// genuinely-down server advertised by its peers would be re-dialled every
	// state ping (~2s), burning five connection attempts each time.
	rediscoverCooldown = 30 * time.Second
)

type AgentClient struct {
	defaultServerAddress   string // Default server address to connect to
	spaceId                string // Space ID for the agent client
	agentToken             string // Agent authentication token (same across all servers in zone)
	serverURL              string // Server URL for API calls (any server in zone)
	credentialsMutex       sync.RWMutex
	serverListMutex        sync.RWMutex
	serverList             map[string]*agentServer
	knownServerAddresses   map[string]bool
	recentlyGaveUp         map[string]time.Time // address -> time the agent gave up on it (rediscovery cooldown)
	firstRegistrationMutex sync.Mutex
	firstRegistration      bool
	keysMutex              sync.Mutex
	lastPublicSSHKeys      []string
	lastPrivateSSHKey      string
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

	// Health check config — received from server at registration
	healthCheckMu            sync.RWMutex
	healthCheckType          string
	healthCheckConfig        string
	healthCheckSkipSSLVerify bool
	healthCheckTimeout       uint32
	healthCheckInterval      uint32
	healthCheckMaxFailures   uint32
	healthCheckAutoRestart   bool

	// Current health status — set by health check runner, read by reportState
	healthMu sync.RWMutex
	healthy  bool

	activityMu            sync.RWMutex
	activityWriteCount    uint32
	activityCreateCount   uint32
	activityDeleteCount   uint32
	activityRenameCount   uint32
	activityDistinctPaths uint32
	lastActivityAtUnix    int64
	methodCallsTotal      atomic.Uint64
	httpRequestsTotal     atomic.Uint64
	tcpConnectionsTotal   atomic.Uint64

	methodMu     sync.RWMutex
	methodServer *methodServerProcess

	// lastReg is the most recently successful method registration. It's
	// re-published to the knot servers whenever the agent (re)connects, so a
	// knot-server restart doesn't lose the space's methods.
	lastRegMu sync.RWMutex
	lastReg   *methods.Registration
}

func NewAgentClient(defaultServerAddress, spaceId string) *AgentClient {
	return &AgentClient{
		defaultServerAddress: defaultServerAddress,
		spaceId:              spaceId,
		serverList:           make(map[string]*agentServer),
		knownServerAddresses: make(map[string]bool),
		recentlyGaveUp:       make(map[string]time.Time),
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
		healthy:              true,
	}
}

// markRediscoverCooldownLocked records that the agent has just given up on the
// given addresses so the discovery loop leaves them alone for rediscoverCooldown.
// The caller must hold serverListMutex for writing.
func (c *AgentClient) markRediscoverCooldownLocked(addresses ...string) {
	now := time.Now()
	for _, address := range addresses {
		c.recentlyGaveUp[address] = now
	}
}

// inRediscoverCooldownLocked reports whether the address was given up on
// recently enough that it should not be re-dialled yet. The caller must hold
// serverListMutex (read or write).
func (c *AgentClient) inRediscoverCooldownLocked(address string) bool {
	gaveUpAt, ok := c.recentlyGaveUp[address]
	if !ok {
		return false
	}
	return time.Since(gaveUpAt) < rediscoverCooldown
}

// clearRediscoverCooldownLocked removes any cooldown entry for the address. The
// caller must hold serverListMutex for writing.
func (c *AgentClient) clearRediscoverCooldownLocked(address string) {
	delete(c.recentlyGaveUp, address)
}

// discoverNewServersLocked filters advertised endpoints down to the ones that
// should be dialled as new servers: those not already known and not within the
// post-give-up cooldown. The caller must hold serverListMutex (read or write).
func (c *AgentClient) discoverNewServersLocked(endpoints []string) []string {
	var newServers []string
	for _, reportedServer := range endpoints {
		if c.knownServerAddresses[reportedServer] {
			continue
		}
		if c.inRediscoverCooldownLocked(reportedServer) {
			continue
		}
		if !stringInSlice(reportedServer, newServers) {
			newServers = append(newServers, reportedServer)
		}
	}
	return newServers
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
	c.knownServerAddresses[connection.address] = true
	connection.ConnectAndServe()

	c.serverListMutex.Unlock()

	// Init log message transport
	go c.initLogMessages()

	// Start periodic status reporting
	go c.reportState()

	// Start health check runner
	go c.RunHealthChecks()
}

func (c *AgentClient) Shutdown() {
	// Stop the method server process first so the exit goroutine can clean
	// up (it will try to unregister from the knot server, which races with
	// the connection close below — that's fine, the server removes the
	// methods when the session drops anyway).
	c.stopMethodServer()

	c.serverListMutex.Lock()
	for _, server := range c.serverList {
		server.Shutdown()
	}
	c.serverList = make(map[string]*agentServer) // Clear the server list
	c.knownServerAddresses = make(map[string]bool)
	c.serverListMutex.Unlock()
}

func (c *AgentClient) GetSpaceId() string {
	return c.spaceId
}

// HasLiveServerConnection reports whether the agent currently has at least one
// knot server connection with a live mux session. Method registration can only
// be published once a session exists (publishMethods fails, without retry, if
// none is connected), so startup registration waits on this.
func (c *AgentClient) HasLiveServerConnection() bool {
	c.serverListMutex.RLock()
	defer c.serverListMutex.RUnlock()
	for _, server := range c.serverList {
		if server.muxSession != nil && !server.muxSession.IsClosed() {
			return true
		}
	}
	return false
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

func (c *AgentClient) snapshotActivityState() (uint32, uint32, uint32, uint32, uint32, int64) {
	c.activityMu.RLock()
	defer c.activityMu.RUnlock()

	return c.activityWriteCount,
		c.activityCreateCount,
		c.activityDeleteCount,
		c.activityRenameCount,
		c.activityDistinctPaths,
		c.lastActivityAtUnix
}
