package msg

import (
	"fmt"

	"github.com/gorilla/websocket"
	"github.com/vmihailenco/msgpack/v5"
)

const (
	MSG_NONE = iota
	MSG_BOOTSTRAP

	MSG_REGISTER // Used when passing registration messages

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
	MSG_SYNC_USER_SPACES

	MSG_UPDATE_VOLUME // Sent to origin server to update a volume

	MSG_MIRROR_TOKEN // Sent to origin to create a token as a mirror to a leaf token
	MSG_DELETE_TOKEN

	MSG_SYNC_ROLES // Sent to origin server to request all roles
	MSG_UPDATE_ROLE
	MSG_DELETE_ROLE
)

// message used internally between the leaf and origin
type LeafOriginMessage struct {
	Command byte
	Payload interface{}
}

type Packet struct {
	Command byte
	payload []byte
}

func WritePacket(ws *websocket.Conn, cmd byte, payload interface{}) error {
	// Serialize the packet using MessagePack
	encodedPacket, err := msgpack.Marshal(payload)
	if err != nil {
		return err
	}

	// Append the command byte to the end of the payload
	encodedPacket = append(encodedPacket, cmd)

	// Write the encoded packet
	return ws.WriteMessage(websocket.BinaryMessage, encodedPacket)
}

func ReadPacket(ws *websocket.Conn) (*Packet, error) {
	msgType, message, err := ws.ReadMessage()
	if err != nil {
		return nil, err
	}

	if msgType != websocket.BinaryMessage {
		return nil, fmt.Errorf("expected binary message, got %d", msgType)
	}

	if len(message) < 1 {
		return nil, fmt.Errorf("received empty message")
	}

	// Extract the command byte and the payload
	command := message[len(message)-1]
	payload := message // The decoder ignores the trailing command byte

	return &Packet{
		Command: command,
		payload: payload,
	}, nil
}

func (p *Packet) UnmarshalPayload(v interface{}) error {
	return msgpack.Unmarshal(p.payload, v)
}
