package msg

import (
	"net"

	"github.com/paularlott/knot/internal/log"
)

type AgentState struct {
	HasCodeServer         bool
	SSHPort               int
	VNCHttpPort           int
	HasTerminal           bool
	TcpPorts              map[string]string
	HttpPorts             map[string]string
	HasVSCodeTunnel       bool
	VSCodeTunnelName      string
	Healthy               bool
	CPUPercent            float64
	MemoryUsedBytes       uint64
	MemoryLimitBytes      uint64
	DiskUsedBytes         uint64
	DiskLimitBytes        uint64
	ActivityWriteCount    uint32
	ActivityCreateCount   uint32
	ActivityDeleteCount   uint32
	ActivityRenameCount   uint32
	ActivityDistinctPaths uint32
	ActivityDistinctDirs  uint32
	LastActivityAtUnix    int64
}

type AgentStateReply struct {
	Endpoints []string
}

func SendState(conn net.Conn, hasCodeServer bool, sshPort int, vncHttpPort int, hasTerminal bool, tcpPorts *map[string]string, httpPorts *map[string]string, hasVSCodeTunnel bool, vscodeTunnelName string, healthy bool, cpuPercent float64, memoryUsedBytes uint64, memoryLimitBytes uint64, diskUsedBytes uint64, diskLimitBytes uint64, activityWriteCount uint32, activityCreateCount uint32, activityDeleteCount uint32, activityRenameCount uint32, activityDistinctPaths uint32, activityDistinctDirs uint32, lastActivityAtUnix int64) (AgentStateReply, error) {
	logger := log.WithGroup("agent")
	err := WriteCommand(conn, CmdUpdateState)
	if err != nil {
		logger.WithError(err).Error("writing state command")
		return AgentStateReply{}, err
	}

	err = WriteMessage(conn, &AgentState{
		HasCodeServer:         hasCodeServer,
		SSHPort:               sshPort,
		VNCHttpPort:           vncHttpPort,
		HasTerminal:           hasTerminal,
		TcpPorts:              *tcpPorts,
		HttpPorts:             *httpPorts,
		HasVSCodeTunnel:       hasVSCodeTunnel,
		VSCodeTunnelName:      vscodeTunnelName,
		Healthy:               healthy,
		CPUPercent:            cpuPercent,
		MemoryUsedBytes:       memoryUsedBytes,
		MemoryLimitBytes:      memoryLimitBytes,
		DiskUsedBytes:         diskUsedBytes,
		DiskLimitBytes:        diskLimitBytes,
		ActivityWriteCount:    activityWriteCount,
		ActivityCreateCount:   activityCreateCount,
		ActivityDeleteCount:   activityDeleteCount,
		ActivityRenameCount:   activityRenameCount,
		ActivityDistinctPaths: activityDistinctPaths,
		ActivityDistinctDirs:  activityDistinctDirs,
		LastActivityAtUnix:    lastActivityAtUnix,
	})
	if err != nil {
		logger.WithError(err).Error("writing state message")
		return AgentStateReply{}, err
	}

	var reply AgentStateReply
	if err := ReadMessage(conn, &reply); err != nil {
		logger.WithError(err).Error("reading agent state reply")
		return AgentStateReply{}, err
	}

	return reply, nil
}
