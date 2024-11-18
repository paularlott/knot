package leaf_server

import (
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/origin_leaf/msg"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

func HandleUpdateTemplateVar(ws *websocket.Conn) error {
	db := database.GetInstance()

	var data model.TemplateVar
	err := msg.ReadMessage(ws, &data)
	if err != nil {
		return err
	}

	log.Debug().Msgf("leaf: updating template var %s - %s", data.Id, data.Name)

	if err := db.SaveTemplateVar(&data); err != nil {
		log.Error().Msgf("error saving template var: %s", err)
		return err
	}

	return nil
}

func HandleDeleteTemplateVar(ws *websocket.Conn) error {
	db := database.GetInstance()

	var id string
	err := msg.ReadMessage(ws, &id)
	if err != nil {
		return err
	}

	// Load the var & delete it
	templateVar, err := db.GetTemplateVar(id)
	if err == nil && templateVar != nil {
		log.Debug().Msgf("leaf: deleting template var %s - %s", templateVar.Id, templateVar.Name)
		db.DeleteTemplateVar(templateVar)
	}

	return nil
}
