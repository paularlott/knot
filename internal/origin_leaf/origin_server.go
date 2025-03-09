package origin_leaf

import (
	"net/http"

	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/origin_leaf/leaf"
	"github.com/paularlott/knot/internal/origin_leaf/msg"
	"github.com/paularlott/knot/internal/origin_leaf/server_info"
	"github.com/paularlott/knot/util"

	"github.com/rs/zerolog/log"
)

func StartLeafSessionGC() {
	go leaf.Gc()
}

func OriginListenAndServe(w http.ResponseWriter, r *http.Request) {

	// If auth was done with an API token then get it as we use it to restrict what the user can do
	token, _ := r.Context().Value("access_token").(*model.Token)

	// Upgrade to a websocket
	ws := util.UpgradeToWS(w, r)
	if ws == nil {
		log.Error().Msg("ws: error while upgrading to websocket")
		return
	}
	defer ws.Close()

	// Wait for the register message
	packet, err := msg.ReadPacket(ws)
	if err != nil {
		log.Error().Msgf("origin: error while reading packet: %s", err)
		return
	}

	// Decode the packet payload
	var registerMsg msg.Register
	err = packet.UnmarshalPayload(&registerMsg)
	if err != nil {
		log.Error().Msgf("origin: error while reading message: %s", err)
		return
	}

	registerReply := msg.RegisterResponse{
		Success:        true,
		RestrictedNode: token != nil,
		Version:        build.Version,
		Location:       registerMsg.Location,
		Timezone:       server_info.Timezone,
	}

	// if using an API token then generate the location
	if token != nil {
		user, err := database.GetInstance().GetUser(token.UserId)
		if err != nil {
			log.Error().Msgf("origin: error while getting user: %s", err)
			return
		}

		registerReply.Location = user.Username
	}

	// If versions mismatch then don't allow the registration
	if registerMsg.Version != build.Version {
		registerReply.Success = false
	}

	// Create or get the leaf session
	leafSession := leaf.Register(registerMsg.SessionId, ws, registerMsg.Location, token)
	registerReply.SessionId = leafSession.Id

	// Write the response
	err = msg.WritePacket(ws, msg.MSG_REGISTER, &registerReply)
	if err != nil {
		log.Error().Msgf("origin: error while writing message: %s", err)
		return
	}

	// Set up the ping handler, used to keep the lead session alive
	ws.SetPingHandler(func(appData string) error {
		leafSession.KeepAlive()
		return nil
	})

	// Loop forever processing messages from the websocket
	for {

		// Read the message packet
		packet, err := msg.ReadPacket(ws)
		if err != nil {
			log.Error().Msgf("origin: error while reading packet: %s", err)
			return
		}

		// Process the command
		switch packet.Command {
		case msg.MSG_BOOTSTRAP:
			leafSession.Bootstrap()

		case msg.MSG_SYNC_TEMPLATES:
			err := originHandleSyncTemplates(packet, leafSession)
			if err != nil {
				log.Error().Msgf("origin: error while handling sync templates: %s", err)
				return
			}

		case msg.MSG_SYNC_USER:
			err := originHandleSyncUser(packet, leafSession, token)
			if err != nil {
				log.Error().Msgf("origin: error while handling sync user: %s", err)
				return
			}

		case msg.MSG_SYNC_TEMPLATEVARS:
			err := originHandleSyncTemplateVars(packet, leafSession)
			if err != nil {
				log.Error().Msgf("origin: error while handling sync template vars: %s", err)
				return
			}

		case msg.MSG_UPDATE_SPACE:
			err := originHandleUpdateSpace(packet, token, leafSession)
			if err != nil {
				log.Error().Msgf("origin: error while handling update space: %s", err)
				return
			}

		case msg.MSG_SYNC_SPACE:
			err := originHandleSyncSpace(packet, leafSession)
			if err != nil {
				log.Error().Msgf("origin: error while handling sync space: %s", err)
				return
			}

		case msg.MSG_SYNC_USER_SPACES:
			err := originHandleSyncUserSpaces(packet, leafSession)
			if err != nil {
				log.Error().Msgf("origin: error while handling sync user spaces: %s", err)
				return
			}

		case msg.MSG_DELETE_SPACE:
			err := originHandleDeleteSpace(packet, token, leafSession)
			if err != nil {
				log.Error().Msgf("origin: error while handling delete space: %s", err)
				return
			}

		case msg.MSG_UPDATE_VOLUME:
			err := originHandleUpdateVolume(packet, token)
			if err != nil {
				log.Error().Msgf("origin: error while handling update volume: %s", err)
				return
			}

		case msg.MSG_MIRROR_TOKEN:
			err := originHandleMirrorToken(packet, token)
			if err != nil {
				log.Error().Msgf("origin: error while handling mirror token: %s", err)
				return
			}

		case msg.MSG_DELETE_TOKEN:
			err := originHandleDeleteToken(packet, token)
			if err != nil {
				log.Error().Msgf("origin: error while handling delete token: %s", err)
				return
			}

		case msg.MSG_SYNC_ROLES:
			err := originHandleSyncRoles(leafSession)
			if err != nil {
				log.Error().Msgf("origin: error while handling sync roles: %s", err)
				return
			}

		default:
			log.Error().Msgf("origin: unknown command: %d", packet.Command)
			return
		}
	}
}

// origin server handler to process sync user messages
func originHandleSyncUser(packet *msg.Packet, session *leaf.Session, token *model.Token) error {
	// Read the message
	var syncUserId string
	err := packet.UnmarshalPayload(&syncUserId)
	if err != nil {
		return err
	}

	db := database.GetInstance()

	// Fetch the user from the database
	user, err := db.GetUser(syncUserId)
	if err != nil {
		// If user not found then tell the follower to delete the user
		if err.Error() == "user not found" {
			log.Debug().Msgf("origin: user %s not found", syncUserId)
			session.DeleteUser(syncUserId)
		} else {
			return err
		}
	} else {
		// for token users only update matching tokens owner
		if token == nil || syncUserId == token.UserId {
			session.UpdateUser(user, nil)
		} else if token != nil {
			log.Warn().Msgf("origin: user %s, not owned by token owner removing", syncUserId)
			session.DeleteUser(syncUserId)
		}
	}

	return nil
}

// origin server handler to process sync space messages
func originHandleSyncSpace(packet *msg.Packet, session *leaf.Session) error {
	// Read the message
	var syncSpaceId string
	err := packet.UnmarshalPayload(&syncSpaceId)
	if err != nil {
		return err
	}

	db := database.GetInstance()

	// Fetch the space from the database
	space, err := db.GetSpace(syncSpaceId)
	if err != nil {
		// If space not found then tell the follower to delete the space
		if err.Error() == "space not found" {
			log.Debug().Msgf("origin: space %s not found", syncSpaceId)
			session.DeleteSpace(syncSpaceId)
		} else {
			return err
		}
	} else {
		if !session.UpdateSpace(space, nil) {
			log.Warn().Msgf("origin: space %s not permitted to sync by token", space.Id)
			session.DeleteSpace(syncSpaceId)
		}
	}

	return nil
}

func originHandleSyncUserSpaces(packet *msg.Packet, session *leaf.Session) error {
	// Read the message
	var data msg.SyncUserSpaces
	err := packet.UnmarshalPayload(&data)
	if err != nil {
		return err
	}

	db := database.GetInstance()

	// Get the spaces for the user
	spaces, err := db.GetSpacesForUser(data.UserId)
	if err != nil {
		return err
	}

	// Loop through the spaces and any not in the existing list send to the leaf
	for _, space := range spaces {
		if space.UserId == data.UserId && !util.InArray(data.Existing, space.Id) {
			// Read the space again to ensure we get the alt names
			s, err := db.GetSpace(space.Id)
			if err == nil && s != nil {
				space = s
			}
			session.UpdateSpace(s, nil)
		}
	}

	return nil
}

// origin server handler to process sync template vars messages
func originHandleSyncTemplateVars(packet *msg.Packet, session *leaf.Session) error {
	// Read the message
	var data msg.SyncTemplateVars
	err := packet.UnmarshalPayload(&data)
	if err != nil {
		return err
	}

	db := database.GetInstance()

	// Get all template variables
	templateVars, err := db.GetTemplateVars()
	if err != nil {
		return err
	}

	// Loop through the template vars and update the leaf, track those in data.Existing not in templateVars to delete
	for _, templateVar := range templateVars {
		if !session.UpdateTemplateVar(templateVar) {
			log.Debug().Msgf("origin: template var %s not permitted to sync", templateVar.Id)
		} else {
			// Remove the template var from the data.Existing
			for i, existing := range data.Existing {
				if existing == templateVar.Id {
					data.Existing = append(data.Existing[:i], data.Existing[i+1:]...)
					break
				}
			}
		}
	}

	// Delete any template vars not in the database
	for _, id := range data.Existing {
		session.DeleteTemplateVar(id)
	}

	return nil
}

// origin server handler to process sync templates messages
func originHandleSyncTemplates(packet *msg.Packet, session *leaf.Session) error {
	log.Debug().Msgf("origin: sync templates")

	// read the message
	var data msg.SyncTemplates
	err := packet.UnmarshalPayload(&data)
	if err != nil {
		return err
	}

	db := database.GetInstance()

	// Get all templates
	templates, err := db.GetTemplates()
	if err != nil {
		return err
	}

	// Loop through the templates and update the leaf, track those in data.Existing not in templates to delete
	for _, template := range templates {
		session.UpdateTemplate(template, nil)

		// Remove the template from the data.Existing
		for i, existing := range data.Existing {
			if existing == template.Id {
				data.Existing = append(data.Existing[:i], data.Existing[i+1:]...)
				break
			}
		}
	}

	// Delete any templates not in the database
	for _, id := range data.Existing {
		session.DeleteTemplate(id)
	}

	return nil
}

// origin server handler to process delete space messages
func originHandleDeleteSpace(packet *msg.Packet, token *model.Token, session *leaf.Session) error {
	// read the message
	var spaceId string
	err := packet.UnmarshalPayload(&spaceId)
	if err != nil {
		return err
	}

	db := database.GetInstance()

	// Fetch the space from the database
	space, err := db.GetSpace(spaceId)
	if err == nil && space != nil && (token == nil || space.UserId == token.UserId) {

		// notify all leaf servers to delete the space
		leaf.DeleteSpace(spaceId, session)

		// Delete the space
		log.Debug().Msgf("origin: deleting space %s - %s", space.Id, space.Name)
		return db.DeleteSpace(space)
	}

	return nil
}

func originHandleUpdateSpace(packet *msg.Packet, token *model.Token, session *leaf.Session) error {
	// read the message
	var updateMsg msg.UpdateSpace
	err := packet.UnmarshalPayload(&updateMsg)
	if err != nil {
		return err
	}

	db := database.GetInstance()

	// Attempt to load the space, only update existing spaces
	existingSpace, err := db.GetSpace(updateMsg.Space.Id)
	if err == nil && existingSpace != nil && existingSpace.UserId == updateMsg.Space.UserId && (token == nil || existingSpace.UserId == token.UserId) {
		log.Debug().Msgf("origin: updating space %s", updateMsg.Space.Id)

		// notify all leaf servers to update the space
		leaf.UpdateSpace(&updateMsg.Space, updateMsg.UpdateFields, session)

		// Update the space in the database
		return db.SaveSpace(&updateMsg.Space, updateMsg.UpdateFields)
	} else if token != nil && existingSpace != nil && existingSpace.UserId != token.UserId {
		// Get the leaf to drop the space as it's out of sync with the origin server
		log.Warn().Msgf("origin: space %s not owned by token owner", updateMsg.Space.Id)
		session.DeleteSpace(updateMsg.Space.Id)
	}

	return nil
}

func originHandleUpdateVolume(packet *msg.Packet, token *model.Token) error {
	// read the message
	var volume msg.UpdateVolume
	err := packet.UnmarshalPayload(&volume)
	if err != nil {
		return err
	}

	// if leaf is using an api token then ignore volume updates
	if token == nil {
		db := database.GetInstance()

		// Attempt to load the volume, only update existing volumes
		existingVolume, err := db.GetVolume(volume.Volume.Id)
		if err == nil && existingVolume != nil {
			log.Debug().Msgf("origin: updating volume %s", volume.Volume.Id)

			// Update the volume in the database
			return db.SaveVolume(&volume.Volume, volume.UpdateFields)
		}
	}

	return nil
}

func originHandleMirrorToken(packet *msg.Packet, accessToken *model.Token) error {
	// read the message
	var token model.Token
	err := packet.UnmarshalPayload(&token)
	if err != nil {
		return err
	}

	// remove the session id
	token.SessionId = ""

	db := database.GetInstance()

	// Check the user exists
	user, err := db.GetUser(token.UserId)
	if err == nil && user != nil && (accessToken == nil || user.Id == accessToken.UserId) {
		// Save the token
		return db.SaveToken(&token)
	}

	return nil
}

func originHandleDeleteToken(packet *msg.Packet, accessToken *model.Token) error {
	// read the message
	var data model.Token
	err := packet.UnmarshalPayload(&data)
	if err != nil {
		return err
	}

	db := database.GetInstance()

	// Fetch the token from the database
	token, err := db.GetToken(data.Id)
	if err == nil && token != nil && token.UserId == data.UserId && (accessToken == nil || token.UserId == accessToken.UserId) {
		log.Debug().Msgf("origin: deleting token %s", token.Id)
		return db.DeleteToken(token)
	}

	return nil
}

// origin server handler to process sync roles messages
func originHandleSyncRoles(session *leaf.Session) error {
	roles := model.GetRolesFromCache()
	for _, role := range roles {
		session.UpdateRole(role)
	}

	return nil
}
