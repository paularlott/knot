package origin

import (
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/origin_leaf/msg"
	"github.com/paularlott/knot/internal/origin_leaf/server_info"
)

var (
	// Channel to send messages to the origin server
	OriginChannel chan *msg.ClientMessage
)

func DeleteSpace(id string) {
	if server_info.IsLeaf {
		message := &msg.ClientMessage{
			Command: msg.MSG_DELETE_SPACE,
			Payload: &id,
		}

		OriginChannel <- message
	}
}

func UpdateSpace(space *model.Space, updateFields []string) {
	if server_info.IsLeaf {
		message := &msg.ClientMessage{
			Command: msg.MSG_UPDATE_SPACE,
			Payload: &msg.UpdateSpace{
				Space:        *space,
				UpdateFields: updateFields,
			},
		}

		OriginChannel <- message
	}
}
