package msg

import (
	"encoding/binary"
	"net"
	"time"

	"github.com/vmihailenco/msgpack/v5"
)

type CmdType byte

const (
	CmdNone CmdType = iota
	CmdPing
	CmdUpdateState
	CmdUpdateAuthorizedKeys
	CmdTerminal
	CmdCodeServer
	CmdProxyTCPPort
	CmdProxyVNC
	CmdProxyHTTP
	CmdVSCodeTunnelTerminal
	CmdLogMessage
	CmdUpdateShell
	CmdUpdateSpaceNote
	CmdUpdateSpaceVar
	CmdGetSpaceVar
	CmdCreateToken
	CmdTunnelPort
	CmdTunnelPortConnection
	CmdSpaceStop
	CmdSpaceRestart
	CmdRunCommand
	CmdCopyFile
	CmdPortForward
	CmdPortList
	CmdPortStop
)

func WriteCommand(conn net.Conn, cmdType CmdType) error {
	_, err := conn.Write([]byte{byte(cmdType)})
	return err
}

func ReadCommand(conn net.Conn) (byte, error) {
	cmdTypeBuf := make([]byte, 1)
	_, err := conn.Read(cmdTypeBuf)
	return cmdTypeBuf[0], err
}

func WriteMessage(conn net.Conn, payload interface{}) error {
	// Serialize the payload using MessagePack
	encodedPayload, err := msgpack.Marshal(payload)
	if err != nil {
		return err
	}

	// Write the size of the payload using binary.BigEndian
	payloadSize := uint32(len(encodedPayload))
	sizeBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(sizeBytes, payloadSize)
	if _, err := conn.Write(sizeBytes); err != nil {
		return err
	}

	// Write the encoded payload
	if _, err := conn.Write(encodedPayload); err != nil {
		return err
	}

	return nil
}

func ReadMessage(conn net.Conn, v interface{}) error {
	// Set a read deadline
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	defer conn.SetReadDeadline(time.Time{})

	// Read the size of the payload
	sizeBytes := make([]byte, 4)
	if _, err := conn.Read(sizeBytes); err != nil {
		return err
	}
	payloadSize := binary.BigEndian.Uint32(sizeBytes)

	// Read the payload
	payloadBuf := make([]byte, payloadSize)
	var totalRead uint32 = 0

	for totalRead < payloadSize {
		n, err := conn.Read(payloadBuf[totalRead:])
		if err != nil {
			return err
		}
		totalRead += uint32(n)
	}

	// Deserialize the payload into v
	if err := msgpack.Unmarshal(payloadBuf, v); err != nil {
		return err
	}

	return nil
}
