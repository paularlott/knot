package origin

import (
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/origin_leaf/msg"
	"github.com/spf13/viper"
)

func UpdateVolume(volume *model.Volume) {
	if viper.GetBool("server.is_leaf") {
		message := &msg.ClientMessage{
			Command: msg.MSG_UPDATE_VOLUME,
			Payload: volume,
		}

		OriginChannel <- message
	}
}
