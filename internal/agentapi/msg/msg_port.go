package msg

import (
	"net"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/log"
)

type TcpPort struct {
	Port uint16
}

type HttpPort struct {
	Port       uint16
	ServerName string
}

// Port forward between spaces
type PortForwardRequest struct {
	LocalPort  uint16 `json:"local_port" msgpack:"local_port"`
	Space      string `json:"space" msgpack:"space"`
	RemotePort uint16 `json:"remote_port" msgpack:"remote_port"`
	Persistent bool   `json:"persistent" msgpack:"persistent"`
	Force      bool   `json:"force" msgpack:"force"`
}

type PortForwardResponse struct {
	Success bool   `json:"success" msgpack:"success"`
	Error   string `json:"error" msgpack:"error"`
}

type PortListResponse struct {
	Forwards []PortForwardInfo `json:"forwards" msgpack:"forwards"`
}

type PortForwardInfo struct {
	LocalPort  uint16 `json:"local_port" msgpack:"local_port"`
	Space      string `json:"space" msgpack:"space"`
	RemotePort uint16 `json:"remote_port" msgpack:"remote_port"`
	Persistent bool   `json:"persistent" msgpack:"persistent"`
}

type PortStopRequest struct {
	LocalPort uint16 `json:"local_port" msgpack:"local_port"`
}

type PortStopResponse struct {
	Success bool   `json:"success" msgpack:"success"`
	Error   string `json:"error" msgpack:"error"`
}

type AddPortForwardMsg struct {
	model.PortForwardEntry
}

type RemovePortForwardMsg struct {
	LocalPort uint16 `json:"local_port" msgpack:"local_port"`
}

func SendAddPortForward(conn net.Conn, entry model.PortForwardEntry) error {
	if err := WriteCommand(conn, CmdAddPortForward); err != nil {
		log.WithError(err).Error("writing add port forward command")
		return err
	}
	if err := WriteMessage(conn, &AddPortForwardMsg{PortForwardEntry: entry}); err != nil {
		log.WithError(err).Error("writing add port forward message")
		return err
	}
	return nil
}

func SendRemovePortForward(conn net.Conn, localPort uint16) error {
	if err := WriteCommand(conn, CmdRemovePortForward); err != nil {
		log.WithError(err).Error("writing remove port forward command")
		return err
	}
	if err := WriteMessage(conn, &RemovePortForwardMsg{LocalPort: localPort}); err != nil {
		log.WithError(err).Error("writing remove port forward message")
		return err
	}
	return nil
}
