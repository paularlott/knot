package origin

import (
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/origin_leaf/msg"
	"github.com/paularlott/knot/internal/origin_leaf/server_info"
)

func MirrorToken(token *model.Token) {
	if server_info.IsLeaf {
		message := &msg.LeafOriginMessage{
			Command: msg.MSG_MIRROR_TOKEN,
			Payload: token,
		}

		OriginChannel <- message
	}
}

func DeleteToken(token *model.Token) {
	if server_info.IsLeaf {
		message := &msg.LeafOriginMessage{
			Command: msg.MSG_DELETE_TOKEN,
			Payload: &token,
		}

		OriginChannel <- message
	}
}
