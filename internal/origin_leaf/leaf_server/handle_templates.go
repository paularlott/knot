package leaf_server

import (
	"github.com/paularlott/knot/api/api_utils"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/internal/origin_leaf/msg"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

// handle template updates sent from the origin server
func HandleUpdateTemplate(ws *websocket.Conn) error {
	var data msg.UpdateTemplate
	err := msg.ReadMessage(ws, &data)
	if err != nil {
		return err
	}

	go func() {
		log.Debug().Msgf("leaf: updating template %s - %s", data.Template.Id, data.Template.Name)

		db := database.GetInstance()
		api_utils.UpdateTemplateHash(data.Template.Id, data.Template.Hash)
		if err := db.SaveTemplate(&data.Template, data.UpdateFields); err != nil {
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
