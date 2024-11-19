package origin

import (
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/origin_leaf/msg"
	"github.com/spf13/viper"
)

func MirrorToken(token *model.Token) {
	if viper.GetBool("server.is_leaf") {
		message := &msg.ClientMessage{
			Command: msg.MSG_MIRROR_TOKEN,
			Payload: token,
		}

		OriginChannel <- message
	}
}

func DeleteToken(token *model.Token) {
	if viper.GetBool("server.is_leaf") {
		message := &msg.ClientMessage{
			Command: msg.MSG_DELETE_TOKEN,
			Payload: &token,
		}

		OriginChannel <- message
	}
}
