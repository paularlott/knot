package msg

import (
	"net"
	"time"

	"github.com/paularlott/knot/internal/log"
)

type AgentState struct {
	HasCodeServer           bool
	SSHPort                 int
	VNCHttpPort             int
	HasTerminal             bool
	TcpPorts                map[string]string
	HttpPorts               map[string]string
	HasVSCodeTunnel         bool
	VSCodeTunnelName        string
	Healthy                 bool
	CPUPercent              float64
	MemoryUsedBytes         uint64
	MemoryLimitBytes        uint64
	DiskUsedBytes           uint64
	DiskLimitBytes          uint64
	ActivityWriteCount      uint32
	ActivityCreateCount     uint32
	ActivityDeleteCount     uint32
	ActivityRenameCount     uint32
	ActivityDistinctPaths   uint32
	ActivityBucketStartUnix int64
	ActivityBucketFinalized bool
	LastActivityAtUnix      int64
	MethodCallsTotal        uint64
	HTTPRequestsTotal       uint64
	TCPConnectionsTotal     uint64
}

type AgentStateReply struct {
	Endpoints []string
}

// stateReplyTimeout caps how long SendState waits for the server's reply. The
// agent reports every couple of seconds; a reply that takes longer than this
// indicates a wedged handler, and we'd rather drop this report (and retry on
// the next tick) than block the reporting loop indefinitely — which would
// silently freeze telemetry and usage sampling for the space.
const stateReplyTimeout = 10 * time.Second

func SendState(conn net.Conn, hasCodeServer bool, sshPort int, vncHttpPort int, hasTerminal bool, tcpPorts *map[string]string, httpPorts *map[string]string, hasVSCodeTunnel bool, vscodeTunnelName string, healthy bool, cpuPercent float64, memoryUsedBytes uint64, memoryLimitBytes uint64, diskUsedBytes uint64, diskLimitBytes uint64, activityWriteCount uint32, activityCreateCount uint32, activityDeleteCount uint32, activityRenameCount uint32, activityDistinctPaths uint32, activityBucketStartUnix int64, activityBucketFinalized bool, lastActivityAtUnix int64, methodCallsTotal uint64, httpRequestsTotal uint64, tcpConnectionsTotal uint64) (AgentStateReply, error) {
	logger := log.WithGroup("agent")
	err := WriteCommand(conn, CmdUpdateState)
	if err != nil {
		logger.WithError(err).Error("writing state command")
		return AgentStateReply{}, err
	}

	err = WriteMessage(conn, &AgentState{
		HasCodeServer:           hasCodeServer,
		SSHPort:                 sshPort,
		VNCHttpPort:             vncHttpPort,
		HasTerminal:             hasTerminal,
		TcpPorts:                *tcpPorts,
		HttpPorts:               *httpPorts,
		HasVSCodeTunnel:         hasVSCodeTunnel,
		VSCodeTunnelName:        vscodeTunnelName,
		Healthy:                 healthy,
		CPUPercent:              cpuPercent,
		MemoryUsedBytes:         memoryUsedBytes,
		MemoryLimitBytes:        memoryLimitBytes,
		DiskUsedBytes:           diskUsedBytes,
		DiskLimitBytes:          diskLimitBytes,
		ActivityWriteCount:      activityWriteCount,
		ActivityCreateCount:     activityCreateCount,
		ActivityDeleteCount:     activityDeleteCount,
		ActivityRenameCount:     activityRenameCount,
		ActivityDistinctPaths:   activityDistinctPaths,
		ActivityBucketStartUnix: activityBucketStartUnix,
		ActivityBucketFinalized: activityBucketFinalized,
		LastActivityAtUnix:      lastActivityAtUnix,
		MethodCallsTotal:        methodCallsTotal,
		HTTPRequestsTotal:       httpRequestsTotal,
		TCPConnectionsTotal:     tcpConnectionsTotal,
	})
	if err != nil {
		logger.WithError(err).Error("writing state message")
		return AgentStateReply{}, err
	}

	// Guard against a missing or slow server reply: without a deadline a
	// wedged server handler would block the agent's state-reporting loop
	// forever. Drop the report on timeout and retry on the next tick instead.
	if err := conn.SetReadDeadline(time.Now().Add(stateReplyTimeout)); err != nil {
		logger.WithError(err).Error("setting state reply deadline")
		return AgentStateReply{}, err
	}
	defer conn.SetReadDeadline(time.Time{}) // clear deadline; conn is reused

	var reply AgentStateReply
	if err := ReadMessage(conn, &reply); err != nil {
		logger.WithError(err).Error("reading agent state reply")
		return AgentStateReply{}, err
	}

	return reply, nil
}
