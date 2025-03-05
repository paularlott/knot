package leaf_server

import (
	"github.com/paularlott/knot/api/api_utils"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/origin_leaf/msg"
	"github.com/rs/zerolog/log"

	"github.com/gorilla/websocket"
)

// handle template updates sent from the origin server
func HandleUpdateTemplate(ws *websocket.Conn) error {
	var data model.Template
	err := msg.ReadMessage(ws, &data)
	if err != nil {
		return err
	}

	go func() {
		log.Debug().Msgf("leaf: updating template %s - %s", data.Id, data.Name)

		db := database.GetInstance()
		api_utils.UpdateTemplateHash(data.Id, data.Hash)
		if err := db.SaveTemplate(&data); err != nil {
			log.Error().Msgf("error saving template: %s", err)
		}
	}()

	return nil
}

// handle template deletes sent from the origin server
func HandleDeleteTemplate(ws *websocket.Conn) error {
	var id string
	err := msg.ReadMessage(ws, &id)
	if err != nil {
		return err
	}

	api_utils.DeleteTemplateHash(id)

	// Load the existing template
	db := database.GetInstance()
	template, err := db.GetTemplate(id)
	if err != nil || template == nil {
		return nil
	}

	// Delete the template
	log.Debug().Msgf("leaf: deleting template %s - %s", template.Id, template.Name)
	return db.DeleteTemplate(template)
}
