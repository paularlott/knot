package leaf_server

import (
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/origin_leaf/msg"

	"github.com/rs/zerolog/log"
)

func HandleUpdateRole(packet *msg.Packet) error {
	var role model.Role
	err := packet.UnmarshalPayload(&role)
	if err != nil {
		return err
	}

	go func() {
		log.Debug().Msgf("leaf: updating role %s", role.Name)
		model.SaveRoleToCache(&role)
	}()

	return nil
}

func HandleDeleteRole(packet *msg.Packet) error {

	var id string
	err := packet.UnmarshalPayload(&id)
	if err != nil {
		return err
	}

	model.DeleteRoleFromCache(id)

	return nil
}
