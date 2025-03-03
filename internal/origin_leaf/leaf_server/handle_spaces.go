package leaf_server

import (
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/origin_leaf/msg"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

func HandleUpdateSpace(ws *websocket.Conn) error {
	db := database.GetInstance()

	var data model.Space
	err := msg.ReadMessage(ws, &data)
	if err != nil {
		return err
	}

	// Check the user for the space is present
	user, err := db.GetUser(data.UserId)
	if err == nil && user != nil {
		log.Debug().Msgf("leaf: updating space %s - %s", data.Id, data.Name)

		if err := db.UpdateSpace(&data); err != nil {
			log.Error().Msgf("error saving space: %s", err)
			return err
		}
	}

	return nil
}

func HandleDeleteSpace(ws *websocket.Conn) error {
	db := database.GetInstance()

	var id string
	err := msg.ReadMessage(ws, &id)
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
