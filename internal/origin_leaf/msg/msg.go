package msg

import (
	"fmt"
	"time"

	"github.com/gorilla/websocket"
	"github.com/vmihailenco/msgpack/v5"
)

const (
	MSG_NONE = iota
	MSG_BOOTSTRAP
	MSG_PING

	MSG_SYNC_TEMPLATES // Sent to origin server to request all templates
	MSG_UPDATE_TEMPLATE
	MSG_DELETE_TEMPLATE

	MSG_SYNC_USER // Sent to origin server to request a user be synced
	MSG_UPDATE_USER
	MSG_DELETE_USER

	MSG_SYNC_TEMPLATEVARS // Sent to origin server to request all template variables
	MSG_UPDATE_TEMPLATEVAR
	MSG_DELETE_TEMPLATEVAR

	MSG_SYNC_SPACE // Sent to origin server to request a space sync back to the leaf
	MSG_UPDATE_SPACE
	MSG_DELETE_SPACE

	MSG_UPDATE_VOLUME // Sent to origin server to update a volume

	MSG_MIRROR_TOKEN // Sent to origin to create a token as a mirror to a leaf token
	MSG_DELETE_TOKEN
)

// message used internally over the leader / follower channels
type ClientMessage struct {
	Command byte
	Payload interface{}
}

func WriteCommand(ws *websocket.Conn, cmdType byte) error {
	return ws.WriteMessage(websocket.BinaryMessage, []byte{cmdType})
}

func ReadCommand(ws *websocket.Conn) (byte, error) {
	_, message, err := ws.ReadMessage()
	if err != nil {
		return 0, err
	}

	if len(message) < 1 {
		return 0, fmt.Errorf("received empty message")
	}

	return message[0], nil
}

func WriteMessage(ws *websocket.Conn, payload interface{}) error {
	// Serialize the payload using MessagePack
	encodedPayload, err := msgpack.Marshal(payload)
	if err != nil {
		return err
	}

	// Write the encoded payload
	return ws.WriteMessage(websocket.BinaryMessage, encodedPayload)
}

func ReadMessage(ws *websocket.Conn, v interface{}) error {
	// Set a read deadline
	ws.SetReadDeadline(time.Now().Add(5 * time.Second))
	defer ws.SetReadDeadline(time.Time{})

	msgType, message, err := ws.ReadMessage()
	if err != nil {
		return err
	}

	if msgType != websocket.BinaryMessage {
		return fmt.Errorf("expected binary message, got %d", msgType)
	}

	// Deserialize the payload into v
	return msgpack.Unmarshal(message, v)
}
