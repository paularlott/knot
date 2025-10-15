package agentlink

import (
	"encoding/binary"
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
	CommandSpaceStop
	CommandSpaceRestart
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
		log.WithError(err).Error("agent: Failed to marshal message")
		return err
	}

	// Encode the message
	data, err := msgpack.Marshal(msg)
	if err != nil {
		log.WithError(err).Error("agent: Failed to marshal message")
		return err
	}

	deadline := time.Now().Add(3 * time.Second)
	if err := conn.SetWriteDeadline(deadline); err != nil {
		log.WithError(err).Error("agent: Failed to set write deadline")
		return err
	}

	// Write the message length then the message
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(data)))

	_, err = conn.Write(lenBuf)
	if err != nil {
		log.WithError(err).Error("agent: Failed to write message length to socket")
		return err
	}

	_, err = conn.Write(data)
	if err != nil {
		log.WithError(err).Error("agent: Failed to write message to socket")
		return err
	}

	return nil
}

func receiveMsg(conn net.Conn) (*CommandMsg, error) {
	lenBuf := make([]byte, 4)

	deadline := time.Now().Add(3 * time.Second)
	if err := conn.SetReadDeadline(deadline); err != nil {
		log.WithError(err).Error("agent: Failed to set read deadline")
		return nil, err
	}

	// Read response length
	_, err := io.ReadFull(conn, lenBuf)
	if err != nil {
		log.WithError(err).Error("agent: Failed to read response length")
		return nil, err
	}

	// Get message length
	msgLen := binary.BigEndian.Uint32(lenBuf)

	// Read the exact message
	buffer := make([]byte, msgLen)
	_, err = io.ReadFull(conn, buffer)
	if err != nil {
		log.WithError(err).Error("agent: Failed to read response")
		return nil, err
	}

	var msg CommandMsg
	err = msgpack.Unmarshal(buffer, &msg)
	if err != nil {
		log.WithError(err).Error("agent: Failed to unmarshal message")
		return nil, err
	}

	return &msg, nil
}

func SendWithResponseMsg(commandType CommandType, payload interface{}, response interface{}) error {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal("agent: Failed to get home directory", "error", err)
	}

	// Check socket path exists
	socketPath := home + "/" + commandSocketPath + "/" + commandSocket
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
