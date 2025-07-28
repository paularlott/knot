package msg

import (
	"net"

	"github.com/rs/zerolog/log"
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
		log.Error().Msgf("agent: writing state command: %v", err)
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
		log.Error().Msgf("agent: writing state message: %v", err)
		return AgentStateReply{}, err
	}

	// Read the reply
	var reply AgentStateReply
	if err := ReadMessage(conn, &reply); err != nil {
		log.Error().Msgf("agent: reading agent state reply: %v", err)
		return AgentStateReply{}, err
	}

	return reply, nil
}
