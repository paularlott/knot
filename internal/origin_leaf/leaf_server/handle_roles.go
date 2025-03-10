package leaf_server

import (
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/origin_leaf/msg"

	"github.com/rs/zerolog/log"
)

func HandleUpdateRole(message *msg.Message) error {
	var role model.Role
	err := message.UnmarshalPayload(&role)
	if err != nil {
		return err
	}

	go func() {
		log.Debug().Msgf("leaf: updating role %s", role.Name)
		model.SaveRoleToCache(&role)
	}()

	return nil
}

func HandleDeleteRole(message *msg.Message) error {

	var id string
	err := message.UnmarshalPayload(&id)
	if err != nil {
		return err
	}

	model.DeleteRoleFromCache(id)

	return nil
}
