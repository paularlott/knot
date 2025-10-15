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
}

type AgentStateReply struct {
	Endpoints []string
}

func SendState(conn net.Conn, hasCodeServer bool, sshPort int, vncHttpPort int, hasTerminal bool, tcpPorts *map[string]string, httpPorts *map[string]string, hasVSCodeTunnel bool, vscodeTunnelName string) (AgentStateReply, error) {
	// Write the state command
	err := WriteCommand(conn, CmdUpdateState)
	if err != nil {
		log.WithError(err).Error("agent: writing state command:")
		return AgentStateReply{}, err
	}

	// Write the state message
	err = WriteMessage(conn, &AgentState{
		HasCodeServer:    hasCodeServer,
		SSHPort:          sshPort,
		VNCHttpPort:      vncHttpPort,
		HasTerminal:      hasTerminal,
		TcpPorts:         *tcpPorts,
		HttpPorts:        *httpPorts,
		HasVSCodeTunnel:  hasVSCodeTunnel,
		VSCodeTunnelName: vscodeTunnelName,
	})
	if err != nil {
		log.WithError(err).Error("agent: writing state message:")
		return AgentStateReply{}, err
	}

	// Read the reply
	var reply AgentStateReply
	if err := ReadMessage(conn, &reply); err != nil {
		log.WithError(err).Error("agent: reading agent state reply:")
		return AgentStateReply{}, err
	}

	return reply, nil
}
