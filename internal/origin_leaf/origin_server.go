package origin_leaf

import (
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/origin_leaf/leaf"
	"github.com/paularlott/knot/internal/origin_leaf/msg"
	"github.com/paularlott/knot/internal/origin_leaf/server_info"
	"github.com/paularlott/knot/util"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

func OriginListenAndServe(w http.ResponseWriter, r *http.Request) {

	// If auth was done with an API token then get it as we use it to restrict what the user can do
	token, _ := r.Context().Value("access_token").(*model.Token)

	// Generate a UUID for the follower
	id, err := uuid.NewV7()
	if err != nil {
		log.Fatal().Msgf("origin: failed to create leaf ID: %s", err)
	}
	leafId := id.String()

	if token != nil {
		log.Info().Msgf("origin: new leaf %s connecting using API token for %s", leafId, token.UserId)
	} else {
		log.Info().Msgf("origin: new leaf %s connecting", leafId)
	}

	// Upgrade to a websocket
	ws := util.UpgradeToWS(w, r)
	if ws == nil {
		log.Error().Msg("ws: error while upgrading to websocket")
		return
	}
	defer ws.Close()

	// Wait for the register message
	var registerMsg msg.Register
	err = msg.ReadMessage(ws, &registerMsg)
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

	// Create a new follower server
	leafSession := leaf.Register(leafId, ws, registerMsg.Location, token)
	defer leaf.Unregister(leafId)

	// Write the response
	err = msg.WriteMessage(ws, &registerReply)
	if err != nil {
		log.Error().Msgf("origin: error while writing message: %s", err)
		return
	}

	// Loop forever processing messages from the websocket
	for {

		// Get the command
		cmd, err := msg.ReadCommand(ws)
		if err != nil {
			log.Error().Msgf("origin: error while reading command: %s", err)
			return
		}

		// Process the command
		switch cmd {
		case msg.MSG_BOOTSTRAP:
			log.Debug().Msg("origin: bootstrap")
			leafSession.Bootstrap()

		case msg.MSG_PING:
			log.Debug().Msg("origin: ping")
			leafSession.Ping()

		case msg.MSG_SYNC_TEMPLATES:
			log.Debug().Msg("origin: sync templates")
			err := originHandleSyncTemplates(ws, leafSession)
			if err != nil {
				log.Error().Msgf("origin: error while handling sync templates: %s", err)
				return
			}

		case msg.MSG_SYNC_USER:
			log.Debug().Msg("origin: sync user")
			err := originHandleSyncUser(ws, leafSession, token)
			if err != nil {
				log.Error().Msgf("origin: error while handling sync user: %s", err)
				return
			}

		case msg.MSG_SYNC_TEMPLATEVARS:
			log.Debug().Msg("origin: sync template variables")
			err := originHandleSyncTemplateVars(ws, leafSession)
			if err != nil {
				log.Error().Msgf("origin: error while handling sync template vars: %s", err)
				return
			}

		case msg.MSG_UPDATE_SPACE:
			log.Debug().Msg("origin: update space")
			err := originHandleUpdateSpace(ws, token)
			if err != nil {
				log.Error().Msgf("origin: error while handling update space: %s", err)
				return
			}

		case msg.MSG_SYNC_SPACE:
			log.Debug().Msg("origin: sync space")
			err := originHandleSyncSpace(ws, leafSession)
			if err != nil {
				log.Error().Msgf("origin: error while handling sync space: %s", err)
				return
			}

		case msg.MSG_DELETE_SPACE:
			log.Debug().Msg("origin: delete space")
			err := originHandleDeleteSpace(ws, token)
			if err != nil {
				log.Error().Msgf("origin: error while handling delete space: %s", err)
				return
			}

		case msg.MSG_UPDATE_VOLUME:
			log.Debug().Msg("origin: update volume")
			err := originHandleUpdateVolume(ws, token)
			if err != nil {
				log.Error().Msgf("origin: error while handling update volume: %s", err)
				return
			}

		case msg.MSG_MIRROR_TOKEN:
			log.Debug().Msg("origin: mirror token")
			err := originHandleMirrorToken(ws, token)
			if err != nil {
				log.Error().Msgf("origin: error while handling mirror token: %s", err)
				return
			}

		case msg.MSG_DELETE_TOKEN:
			log.Debug().Msg("origin: delete token")
			err := originHandleDeleteToken(ws, token)
			if err != nil {
				log.Error().Msgf("origin: error while handling delete token: %s", err)
				return
			}

		default:
			log.Error().Msgf("origin: unknown command: %d", cmd)
			return
		}
	}
}

// origin server handler to process sync user messages
func originHandleSyncUser(ws *websocket.Conn, session *leaf.Session, token *model.Token) error {
	// Read the message
	var syncUserId string
	err := msg.ReadMessage(ws, &syncUserId)
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
			session.UpdateUser(user)
		} else if token != nil {
			log.Warn().Msgf("origin: user %s, not owned by token owner removing", syncUserId)
			session.DeleteUser(syncUserId)
		}
	}

	return nil
}

// origin server handler to process sync space messages
func originHandleSyncSpace(ws *websocket.Conn, session *leaf.Session) error {
	// Read the message
	var syncSpaceId string
	err := msg.ReadMessage(ws, &syncSpaceId)
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
		if !session.UpdateSpace(space) {
			log.Warn().Msgf("origin: space %s not permitted to sync by token", space.Id)
			session.DeleteSpace(syncSpaceId)
		}
	}

	return nil
}

// origin server handler to process sync template vars messages
func originHandleSyncTemplateVars(ws *websocket.Conn, session *leaf.Session) error {
	// Read the message
	var data msg.SyncTemplateVars
	err := msg.ReadMessage(ws, &data)
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
func originHandleSyncTemplates(ws *websocket.Conn, session *leaf.Session) error {
	// read the message
	var data msg.SyncTemplates
	err := msg.ReadMessage(ws, &data)
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
		session.UpdateTemplate(template)

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
func originHandleDeleteSpace(ws *websocket.Conn, token *model.Token) error {
	// read the message
	var spaceId string
	err := msg.ReadMessage(ws, &spaceId)
	if err != nil {
		return err
	}

	db := database.GetInstance()

	// Fetch the space from the database
	space, err := db.GetSpace(spaceId)
	if err == nil && space != nil && (token == nil || space.UserId == token.UserId) {

		// notify all leaf servers to delete the space
		leaf.DeleteSpace(spaceId)

		// Delete the space
		log.Debug().Msgf("origin: deleting space %s - %s", space.Id, space.Name)
		return db.DeleteSpace(space)
	}

	return nil
}

func originHandleUpdateSpace(ws *websocket.Conn, token *model.Token) error {
	// read the message
	var space model.Space
	err := msg.ReadMessage(ws, &space)
	if err != nil {
		return err
	}

	db := database.GetInstance()

	// Attempt to load the space, only update existing spaces
	existingSpace, err := db.GetSpace(space.Id)
	if err == nil && existingSpace != nil && existingSpace.UserId == space.UserId && (token == nil || existingSpace.UserId == token.UserId) {
		log.Debug().Msgf("origin: updating space %s", space.Id)

		// notify all leaf servers to update the space
		leaf.UpdateSpace(&space)

		// Update the space in the database
		return db.SaveSpace(&space)
	} else if token != nil && existingSpace != nil && existingSpace.UserId != token.UserId {
		log.Warn().Msgf("origin: space %s not owned by token owner", space.Id)
		leaf.DeleteSpace(space.Id)
	}

	return nil
}

func originHandleUpdateVolume(ws *websocket.Conn, token *model.Token) error {
	// read the message
	var volume model.Volume
	err := msg.ReadMessage(ws, &volume)
	if err != nil {
		return err
	}

	// if leaf is using an api token then ignore volume updates
	if token == nil {
		db := database.GetInstance()

		// Attempt to load the volume, only update existing volumes
		existingVolume, err := db.GetVolume(volume.Id)
		if err == nil && existingVolume != nil {
			log.Debug().Msgf("origin: updating volume %s", volume.Id)

			// Update the volume in the database
			return db.SaveVolume(&volume)
		}
	}

	return nil
}

func originHandleMirrorToken(ws *websocket.Conn, accessToken *model.Token) error {
	// read the message
	var token model.Token
	err := msg.ReadMessage(ws, &token)
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

func originHandleDeleteToken(ws *websocket.Conn, accessToken *model.Token) error {
	// read the message
	var data model.Token
	err := msg.ReadMessage(ws, &data)
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
