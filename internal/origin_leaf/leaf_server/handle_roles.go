package leaf_server

import (
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/origin_leaf/msg"
	"github.com/rs/zerolog/log"

	"github.com/gorilla/websocket"
)

func HandleUpdateRole(ws *websocket.Conn) error {
	var role model.Role
	err := msg.ReadMessage(ws, &role)
	if err != nil {
		return err
	}

	go func() {
		log.Debug().Msgf("leaf: updating role %s", role.Name)
		model.SaveRoleToCache(&role)
	}()

	return nil
}

func HandleDeleteRole(ws *websocket.Conn) error {

	var id string
	err := msg.ReadMessage(ws, &id)
	if err != nil {
		return err
	}

	model.DeleteRoleFromCache(id)

	return nil
}
