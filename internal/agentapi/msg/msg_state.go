package msg

import (
	"net"

	"github.com/paularlott/knot/internal/log"
)

type AgentState struct {
	HasCodeServer    bool
	SSHPort          int
	VNCHttpPort      int
	HasTerminal      bool
	TcpPorts         map[string]string
	HttpPorts        map[string]string
	HasVSCodeTunnel  bool
	VSCodeTunnelName string
	Healthy          bool
}

type AgentStateReply struct {
	Endpoints []string
}

func SendState(conn net.Conn, hasCodeServer bool, sshPort int, vncHttpPort int, hasTerminal bool, tcpPorts *map[string]string, httpPorts *map[string]string, hasVSCodeTunnel bool, vscodeTunnelName string, healthy bool) (AgentStateReply, error) {
	logger := log.WithGroup("agent")
	err := WriteCommand(conn, CmdUpdateState)
	if err != nil {
		logger.WithError(err).Error("writing state command")
		return AgentStateReply{}, err
	}

	err = WriteMessage(conn, &AgentState{
		HasCodeServer:    hasCodeServer,
		SSHPort:          sshPort,
		VNCHttpPort:      vncHttpPort,
		HasTerminal:      hasTerminal,
		TcpPorts:         *tcpPorts,
		HttpPorts:        *httpPorts,
		HasVSCodeTunnel:  hasVSCodeTunnel,
		VSCodeTunnelName: vscodeTunnelName,
		Healthy:          healthy,
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
