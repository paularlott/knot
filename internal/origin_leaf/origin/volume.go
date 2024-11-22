package origin

import (
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/origin_leaf/msg"
	"github.com/paularlott/knot/internal/origin_leaf/server_info"
)

func UpdateVolume(volume *model.Volume) {
	if server_info.IsLeaf && !server_info.RestrictedLeaf {
		message := &msg.ClientMessage{
			Command: msg.MSG_UPDATE_VOLUME,
			Payload: volume,
		}

		OriginChannel <- message
	}
}
