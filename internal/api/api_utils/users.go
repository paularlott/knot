package api_utils

import (
	"time"

	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"

	"github.com/rs/zerolog/log"
)

type ApiUtilsUsers struct {
}

func NewApiUtilsUsers() *ApiUtilsUsers {
	return &ApiUtilsUsers{}
}

func (auu *ApiUtilsUsers) DeleteUser(toDelete *model.User) error {
	var hasError = false

	db := database.GetInstance()
	cfg := config.GetServerConfig()

	log.Debug().Msgf("delete user: Deleting user %s", toDelete.Id)

	// Stop all spaces and delete all volumes
	spaces, err := db.GetSpacesForUser(toDelete.Id)
	if err != nil {
		return err
	}

	for _, space := range spaces {
		log.Debug().Msgf("delete user: Deleting space %s", space.Id)

		// Skip spaces shared with the user but not owned by the user
		if space.UserId == toDelete.Id && space.Zone == cfg.Zone {
			log.Debug().Msgf("delete user: Deleting space %s", space.Id)
			service.GetContainerService().DeleteSpace(space)
		}

		space.IsDeleted = true
		space.UpdatedAt = time.Now().UTC()
		db.SaveSpace(space, []string{"IsDeleted", "UpdatedAt"})
		service.GetTransport().GossipSpace(space)
	}

	// Delete the user
	if !hasError {
		log.Debug().Msgf("delete user: Deleting user %s from database", toDelete.Id)
		toDelete.IsDeleted = true
		toDelete.Active = false
		toDelete.Username = toDelete.Id
		toDelete.Email = toDelete.Id
		toDelete.UpdatedAt = time.Now().UTC()
		err = db.SaveUser(toDelete, []string{"IsDeleted", "Active", "UpdatedAt", "Username", "Email"})
		if err != nil {
			return err
		}

		service.GetTransport().GossipUser(toDelete)
		auu.RemoveUsersSessions(toDelete)
		auu.RemoveUsersTokens(toDelete)
	}

	return nil
}

// Delete the sessions owned by a user
func (auu *ApiUtilsUsers) RemoveUsersSessions(user *model.User) {
	store := database.GetSessionStorage()

	// Find sessions for the user and delete them
	sessions, err := store.GetSessionsForUser(user.Id)
	if err == nil && sessions != nil {
		for _, session := range sessions {
			session.IsDeleted = true
			session.ExpiresAfter = time.Now().Add(model.SessionExpiryDuration).UTC()
			session.UpdatedAt = time.Now().UTC()
			store.SaveSession(session)
			service.GetTransport().GossipSession(session)
		}
	}
}

// Delete the tokens owned by a user
func (auu *ApiUtilsUsers) RemoveUsersTokens(user *model.User) {
	db := database.GetInstance()

	// Find API tokens for the user and delete them
	tokens, err := db.GetTokensForUser(user.Id)
	if err == nil && tokens != nil {
		for _, token := range tokens {
			db.DeleteToken(token)
		}
	}
}

func (auu *ApiUtilsUsers) UpdateSpacesSSHKey(user *model.User) {
	db := database.GetInstance()

	log.Debug().Msgf("Updating agent SSH key for user %s", user.Id)

	// Load the list of spaces for the user
	spaces, err := db.GetSpacesForUser(user.Id)
	if err != nil {
		log.Debug().Msgf("Failed to get spaces for user %s: %s", user.Id, err)
		return
	}

	// Loop through all spaces updating the active ones
	for _, space := range spaces {
		auu.UpdateSpaceSSHKeys(space, user)
	}

	log.Debug().Msgf("Finished updating agent SSH key for user %s", user.Id)
}

func (auu *ApiUtilsUsers) UpdateSpaceSSHKeys(space *model.Space, user *model.User) {
	db := database.GetInstance()
	cfg := config.GetServerConfig()

	template, err := db.GetTemplate(space.TemplateId)
	if err != nil || template == nil {
		log.Debug().Msgf("Update SSH Keys: Failed to get template for space %s: %s", space.Id, err)
		return
	}

	if space.IsDeployed || template.IsManual() {
		// Get the agent state
		agentState := agent_server.GetSession(space.Id)
		if agentState == nil {
			// Silently ignore if space is on a different server
			if space.Zone == "" || space.Zone == cfg.Zone {
				log.Debug().Msgf("Update SSH Keys: Agent state not found for space %s", space.Id)
			}

			return
		}

		// If agent accepting SSH keys then update
		if agentState.SSHPort > 0 {
			keys := []string{}
			usernames := []string{}

			// Add the given users keys
			if user.SSHPublicKey != "" {
				keys = append(keys, user.SSHPublicKey)
			}
			if user.GitHubUsername != "" {
				usernames = append(usernames, user.GitHubUsername)
			}

			// If space is shared then get the other users keys
			if space.SharedWithUserId != "" {
				var uid string

				if space.UserId == user.Id {
					uid = space.SharedWithUserId
				} else {
					uid = space.UserId
				}

				other, err := db.GetUser(uid)
				if err == nil && other != nil {
					if other.SSHPublicKey != "" {
						keys = append(keys, other.SSHPublicKey)
					}
					if other.GitHubUsername != "" {
						usernames = append(usernames, other.GitHubUsername)
					}
				}
			}

			log.Debug().Msgf("Sending SSH public key to agent %s", space.Id)
			if err := agentState.SendUpdateAuthorizedKeys(keys, usernames); err != nil {
				log.Debug().Msgf("Failed to send SSH public key to agent: %s", err)
			}
		}
	}
}

// For disabled users ensure all spaces are stopped, for enabled users update the SSH key on the agents
func (auu *ApiUtilsUsers) UpdateUserSpaces(user *model.User) {
	cfg := config.GetServerConfig()

	// If the user is disabled then stop all spaces
	if !user.Active {
		spaces, err := database.GetInstance().GetSpacesForUser(user.Id)
		if err != nil {
			return
		}

		for _, space := range spaces {
			// Skip over spaces shared with the user but not owned by them
			if space.UserId == user.Id && space.IsDeployed && (space.Zone == "" || space.Zone == cfg.Zone) {
				service.GetContainerService().StopSpace(space)
			}
		}

		// Kill the sessions to logout the user, but leave the tokens there until they expire
		auu.RemoveUsersSessions(user)
	} else {
		// Update the SSH key on the agents
		auu.UpdateSpacesSSHKey(user)
	}
}

// Make sure ApiUtilsUsers implements service.UserService
var _ service.UserService = (*ApiUtilsUsers)(nil)
