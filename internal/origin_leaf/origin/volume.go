package origin

import (
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/origin_leaf/msg"
)

func UpdateVolume(volume *model.Volume) {
	if IsLeaf && !RestrictedLeaf {
		message := &msg.ClientMessage{
			Command: msg.MSG_UPDATE_VOLUME,
			Payload: volume,
		}

		OriginChannel <- message
	}
}
