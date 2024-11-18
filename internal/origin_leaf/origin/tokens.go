package origin

import (
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/origin_leaf/msg"
)

func MirrorToken(token *model.Token) {
	message := &msg.ClientMessage{
		Command: msg.MSG_MIRROR_TOKEN,
		Payload: token,
	}

	OriginChannel <- message
}

func DeleteToken(token *model.Token) {
	message := &msg.ClientMessage{
		Command: msg.MSG_DELETE_TOKEN,
		Payload: &token,
	}

	OriginChannel <- message
}
