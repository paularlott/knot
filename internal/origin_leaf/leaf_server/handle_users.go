package leaf_server

import (
	"github.com/paularlott/knot/api/api_utils"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/internal/origin_leaf/msg"

	"github.com/rs/zerolog/log"
)

// handle update messages sent from the origin server
func HandleUpdateUser(packet *msg.Packet) error {
	db := database.GetInstance()

	var userData msg.UpdateUser
	err := packet.UnmarshalPayload(&userData)
	if err != nil {
		return err
	}

	go func() {
		// If the user isn't active then delete it
		if !userData.User.Active {
			log.Debug().Msgf("leaf: removing inactive user %s - %s", userData.User.Id, userData.User.Username)

			// Load the user & delete it
			user, err := db.GetUser(userData.User.Id)
			if err == nil && user != nil {
				log.Debug().Msgf("leaf: deleting user %s - %s", user.Id, user.Username)
				api_utils.DeleteUser(db, user)
			}
		} else {
			// Attempt to load the user, only update existing users
			user, err := db.GetUser(userData.User.Id)
			if err == nil && user != nil {
				log.Debug().Msgf("leaf: updating user %s - %s", userData.User.Id, userData.User.Username)

				// Update the user in the database
				err = db.SaveUser(&userData.User, userData.UpdateFields)
				if err != nil {
					log.Error().Msgf("error saving user: %s", err)
					return
				}

				// Update the user's spaces, ssh keys or stop spaces
				api_utils.UpdateUserSpaces(&userData.User)
			}
		}
	}()

	return nil
}

func HandleDeleteUser(packet *msg.Packet) error {
	db := database.GetInstance()

	var id string
	err := packet.UnmarshalPayload(&id)
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
