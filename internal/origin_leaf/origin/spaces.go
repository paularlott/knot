package origin

import (
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/origin_leaf/msg"
)

var (
	// Channel to send messages to the origin server
	OriginChannel chan *msg.ClientMessage
)

func DeleteSpace(id string) {
	message := &msg.ClientMessage{
		Command: msg.MSG_DELETE_SPACE,
		Payload: &id,
	}

	OriginChannel <- message
}

func UpdateSpace(space *model.Space) {
	message := &msg.ClientMessage{
		Command: msg.MSG_UPDATE_SPACE,
		Payload: space,
	}

	OriginChannel <- message
}
