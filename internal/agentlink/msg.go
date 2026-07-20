package agentlink

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"os"
	"time"

	"github.com/paularlott/knot/internal/log"
	"github.com/vmihailenco/msgpack/v5"
)

type CommandType int

const (
	CommandNil CommandType = iota
	CommandConnect
	CommandSpaceNote
	CommandSpaceSetField
	CommandSpaceGetField
	CommandSpaceStop
	CommandSpaceRestart
	CommandForwardPort
	CommandListPortForwards
	CommandStopPortForward
	CommandRegisterMethods
	CommandRegisterMethodsTOML
	CommandRegisterMethodsScript
	CommandUnregisterMethods
	CommandLog
	CommandStartTunnel
	CommandStopTunnel
	CommandListTunnels
	CommandThrottlePort
)

type CommandMsg struct {
	Command CommandType `msgpack:"c"`
	Payload []byte      `msgpack:"p"`
}

func (c *CommandMsg) Unmarshal(v interface{}) error {
	return msgpack.Unmarshal(c.Payload, v)
}

func sendMsg(conn net.Conn, commandType CommandType, payload interface{}) error {
	var err error

	msg := CommandMsg{
		Command: commandType,
	}

	msg.Payload, err = msgpack.Marshal(payload)
	if err != nil {
		log.WithError(err).Error("Failed to marshal message")
		return err
	}

	// Encode the message
	data, err := msgpack.Marshal(msg)
	if err != nil {
		log.WithError(err).Error("Failed to marshal message")
		return err
	}

	deadline := time.Now().Add(3 * time.Second)
	if err := conn.SetWriteDeadline(deadline); err != nil {
		log.WithError(err).Error("Failed to set write deadline")
		return err
	}

	// Write the message length then the message
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(data)))

	_, err = conn.Write(lenBuf)
	if err != nil {
		log.WithError(err).Error("Failed to write message length to socket")
		return err
	}

	_, err = conn.Write(data)
	if err != nil {
		log.WithError(err).Error("Failed to write message to socket")
		return err
	}

	return nil
}

func receiveMsg(conn net.Conn) (*CommandMsg, error) {
	return receiveMsgTimeout(conn, 3*time.Second)
}

// receiveMsgTimeout reads one length-prefixed command message. A timeout of 0
// clears the read deadline, allowing persistent connections (such as the log
// stream) to block until the peer sends or disconnects.
func receiveMsgTimeout(conn net.Conn, timeout time.Duration) (*CommandMsg, error) {
	lenBuf := make([]byte, 4)

	if timeout > 0 {
		if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
			log.WithError(err).Error("Failed to set read deadline")
			return nil, err
		}
	} else {
		if err := conn.SetReadDeadline(time.Time{}); err != nil {
			return nil, err
		}
	}

	// Read response length
	_, err := io.ReadFull(conn, lenBuf)
	if err != nil {
		return nil, err
	}

	// Get message length
	msgLen := binary.BigEndian.Uint32(lenBuf)

	// Read the exact message
	buffer := make([]byte, msgLen)
	_, err = io.ReadFull(conn, buffer)
	if err != nil {
		return nil, err
	}

	var msg CommandMsg
	err = msgpack.Unmarshal(buffer, &msg)
	if err != nil {
		log.WithError(err).Error("Failed to unmarshal message")
		return nil, err
	}

	return &msg, nil
}

// commandSocketFile returns the absolute path of the agent command socket in
// the user's home directory, or "" if the home directory cannot be resolved.
func commandSocketFile() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home + "/" + commandSocketPath + "/" + commandSocket
}

func IsAgentRunning() bool {
	socketPath := commandSocketFile()
	if socketPath == "" {
		return false
	}
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		return false
	}

	// Just check if we can connect to the socket
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		os.Remove(socketPath)
		return false
	}
	conn.Close()

	return true
}

func SendWithResponseMsg(commandType CommandType, payload interface{}, response interface{}) error {
	// Check socket path exists
	socketPath := commandSocketFile()
	if socketPath == "" {
		return errors.New("failed to get home directory")
	}
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		return err
	}

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return err
	}
	defer conn.Close()

	err = sendMsg(conn, commandType, payload)
	if err != nil {
		return err
	}

	if response != nil {
		// Read response length
		msgRec, err := receiveMsg(conn)
		if err != nil {
			return err
		}

		err = msgRec.Unmarshal(response)
		if err != nil {
			return err
		}
	}

	return nil
}
