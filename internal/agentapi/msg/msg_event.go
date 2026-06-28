package msg

import (
	"net"

	"github.com/paularlott/knot/internal/log"
)

type Event struct {
	EventId   string
	EventType string
	SpaceId   string
	UserId    string
	Payload   []byte
}

type EventReply struct{}

func SendEvent(conn net.Conn, event *Event) error {
	logger := log.WithGroup("agent")

	err := WriteCommand(conn, CmdEvent)
	if err != nil {
		logger.WithError(err).Error("writing event command")
		return err
	}

	err = WriteMessage(conn, event)
	if err != nil {
		logger.WithError(err).Error("writing event message")
		return err
	}

	var reply EventReply
	if err := ReadMessage(conn, &reply); err != nil {
		logger.WithError(err).Error("reading event reply")
		return err
	}

	return nil
}
