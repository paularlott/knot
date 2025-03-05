package leaf_server

import (
	"github.com/paularlott/knot/api/api_utils"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/origin_leaf/msg"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

// handle update messages sent from the origin server
func HandleUpdateUser(ws *websocket.Conn) error {
	db := database.GetInstance()

	var userData model.User
	err := msg.ReadMessage(ws, &userData)
	if err != nil {
		return err
	}

	go func() {
		// If the user isn't active then delete it
		if !userData.Active {
			log.Debug().Msgf("leaf: removing inactive user %s - %s", userData.Id, userData.Username)

			// Load the user & delete it
			user, err := db.GetUser(userData.Id)
			if err == nil && user != nil {
				log.Debug().Msgf("leaf: deleting user %s - %s", user.Id, user.Username)
				api_utils.DeleteUser(db, user)
			}
		} else {
			// Attempt to load the user, only update existing users
			user, err := db.GetUser(userData.Id)
			if err == nil && user != nil {
				log.Debug().Msgf("leaf: updating user %s - %s", userData.Id, userData.Username)

				// Update the user in the database
				err = db.SaveUser(&userData)
				if err != nil {
					log.Error().Msgf("error saving user: %s", err)
					return
				}

				// Update the user's spaces, ssh keys or stop spaces
				api_utils.UpdateUserSpaces(&userData)
			}
		}
	}()

	return nil
}

func HandleDeleteUser(ws *websocket.Conn) error {
	db := database.GetInstance()

	var id string
	err := msg.ReadMessage(ws, &id)
	if err != nil {
		return err
	}

	// Load the user & delete it
	user, err := db.GetUser(id)
	if err == nil && user != nil {
		log.Debug().Msgf("leaf: deleting user %s - %s", user.Id, user.Username)
		api_utils.DeleteUser(db, user)
	}

	return nil
}
