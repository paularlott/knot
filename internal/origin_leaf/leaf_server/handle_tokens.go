package leaf_server

import (
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/internal/origin_leaf/msg"

	"github.com/rs/zerolog/log"
)

func HandleDeleteToken(packet *msg.Packet) error {
	db := database.GetInstance()

	var id string
	err := packet.UnmarshalPayload(&id)
	if err != nil {
		return err
	}

	// Load the token & delete it
	token, err := db.GetToken(id)
	if err == nil && token != nil {
		log.Debug().Msgf("leaf: deleting token %s - %s", token.Id, token.Name)

		db.DeleteToken(token)
	}

	return nil
}
