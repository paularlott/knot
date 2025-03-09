package leaf_server

import (
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/internal/origin_leaf/msg"

	"github.com/rs/zerolog/log"
)

func HandleUpdateSpace(packet *msg.Packet) error {
	db := database.GetInstance()

	var data msg.UpdateSpace
	err := packet.UnmarshalPayload(&data)
	if err != nil {
		return err
	}

	go func() {
		// Check the user for the space is present
		user, err := db.GetUser(data.Space.UserId)
		if err == nil && user != nil {
			log.Debug().Msgf("leaf: updating space %s - %s", data.Space.Id, data.Space.Name)

			if err := db.SaveSpace(&data.Space, data.UpdateFields); err != nil {
				log.Error().Msgf("error saving space: %s", err)
			}
		}
	}()

	return nil
}

func HandleDeleteSpace(packet *msg.Packet) error {
	db := database.GetInstance()

	var id string
	err := packet.UnmarshalPayload(&id)
	if err != nil {
		return err
	}

	// Load the space & delete it
	space, err := db.GetSpace(id)
	if err == nil && space != nil {
		log.Debug().Msgf("leaf: deleting space %s - %s", space.Id, space.Name)

		// only need to remove from the database as the node running the space will have done the shutdown of the jobs
		db.DeleteSpace(space)
	}

	return nil
}
