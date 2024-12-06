package msg

import (
	"net"

	"github.com/rs/zerolog/log"
)

type AgentState struct {
	SpaceId          string
	HasCodeServer    bool
	SSHPort          int
	VNCHttpPort      int
	HasTerminal      bool
	TcpPorts         map[string]string
	HttpPorts        map[string]string
	HasVSCodeTunnel  bool
	VSCodeTunnelName string
	AgentIp          string
}

func SendState(conn net.Conn, spaceId string, hasCodeServer bool, sshPort int, vncHttpPort int, hasTerminal bool, tcpPorts *map[string]string, httpPorts *map[string]string, hasVSCodeTunnel bool, vscodeTunnelName string, ip string) error {
	// Write the state command
	err := WriteCommand(conn, CmdUpdateState)
	if err != nil {
		log.Error().Msgf("agent: writing state command: %v", err)
		return err
	}

	// Write the state message
	err = WriteMessage(conn, &AgentState{
		SpaceId:          spaceId,
		HasCodeServer:    hasCodeServer,
		SSHPort:          sshPort,
		VNCHttpPort:      vncHttpPort,
		HasTerminal:      hasTerminal,
		TcpPorts:         *tcpPorts,
		HttpPorts:        *httpPorts,
		HasVSCodeTunnel:  hasVSCodeTunnel,
		VSCodeTunnelName: vscodeTunnelName,
		AgentIp:          ip,
	})
	if err != nil {
		log.Error().Msgf("agent: writing state message: %v", err)
		return err
	}

	return nil
}
