package leafmsg

import (
	"fmt"

	"github.com/gorilla/websocket"
	"github.com/vmihailenco/msgpack/v5"
)

type MessageType byte

const (
	MessageRegister MessageType = iota
	MessageFullSync
	MessageFullSyncEnd
	MessageGossipGroup
	MessageGossipRole
	MessageGossipTemplate
	MessageGossipTemplateVar
	MessageGossipUser
)

type Message struct {
	Type    MessageType `json:"type" msgpack:"type"`
	payload []byte      `json:"payload" msgpack:"payload"`
}

func (m *Message) UnmarshalPayload(v interface{}) error {
	return msgpack.Unmarshal(m.payload, v)
}

func WriteMessage(ws *websocket.Conn, msgType MessageType, payload interface{}) error {
	encoded, err := msgpack.Marshal(payload)
	if err != nil {
		return err
	}

	encoded = append(encoded, byte(msgType))
	return ws.WriteMessage(websocket.BinaryMessage, encoded)
}

func ReadMessage(ws *websocket.Conn) (*Message, error) {
	messageType, data, err := ws.ReadMessage()
	if err != nil {
		return nil, err
	}

	if messageType != websocket.BinaryMessage {
		return nil, fmt.Errorf("expected binary message, got %d", messageType)
	}

	msg := &Message{
		Type:    MessageType(data[len(data)-1]),
		payload: data[:len(data)-1],
	}

	return msg, nil
}
