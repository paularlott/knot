package origin

import (
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/origin_leaf/msg"
	"github.com/spf13/viper"
)

var (
	// Channel to send messages to the origin server
	OriginChannel chan *msg.ClientMessage
)

func DeleteSpace(id string) {
	if viper.GetBool("server.is_leaf") {
		message := &msg.ClientMessage{
			Command: msg.MSG_DELETE_SPACE,
			Payload: &id,
		}

		OriginChannel <- message
	}
}

func UpdateSpace(space *model.Space) {
	if viper.GetBool("server.is_leaf") {
		message := &msg.ClientMessage{
			Command: msg.MSG_UPDATE_SPACE,
			Payload: space,
		}

		OriginChannel <- message
	}
}
