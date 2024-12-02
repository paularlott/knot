package api_utils

import (
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/container/docker"
	"github.com/paularlott/knot/internal/container/nomad"
	"github.com/paularlott/knot/internal/origin_leaf/server_info"

	"github.com/rs/zerolog/log"
)

func DeleteUser(db database.IDbDriver, toDelete *model.User) error {
	var hasError = false

	log.Debug().Msgf("delete user: Deleting user %s", toDelete.Id)

	// Stop all spaces and delete all volumes
	spaces, err := db.GetSpacesForUser(toDelete.Id)
	if err != nil {
		return err
	}

	// Get the nomad client
	nomadClient := nomad.NewClient()
	containerClient := docker.NewClient()
	for _, space := range spaces {
		log.Debug().Msgf("delete user: Deleting space %s", space.Id)

		if space.Location == server_info.LeafLocation {
			log.Debug().Msgf("delete user: Deleting space %s from nomad", space.Id)

			// Load the space template
			template, err := db.GetTemplate(space.TemplateId)
			if err != nil {
				log.Debug().Msgf("delete user: Failed to get template for space %s: %s", space.Id, err)
				hasError = true
				break
			}

			if template.LocalContainer {
				// Stop the job
				if space.IsDeployed {
					err = containerClient.DeleteSpaceJob(space)
					if err != nil {
						log.Debug().Msgf("delete user: Failed to delete space job %s: %s", space.Id, err)
						hasError = true
						break
					}
				}

				// Delete the volumes
				err = containerClient.DeleteSpaceVolumes(space)
				if err != nil {
					log.Debug().Msgf("delete user: Failed to delete space volumes %s: %s", space.Id, err)
					hasError = true
					break
				}
			} else {
				// Stop the job
				if space.IsDeployed {
					err = nomadClient.DeleteSpaceJob(space)
					if err != nil {
						log.Debug().Msgf("delete user: Failed to delete space job %s: %s", space.Id, err)
						hasError = true
						break
					}
				}

				// Delete the volumes
				err = nomadClient.DeleteSpaceVolumes(space)
				if err != nil {
					log.Debug().Msgf("delete user: Failed to delete space volumes %s: %s", space.Id, err)
					hasError = true
					break
				}
			}
		}

		db.DeleteSpace(space)
	}

	// Delete the user
	if !hasError {
		log.Debug().Msgf("delete user: Deleting user %s from database", toDelete.Id)
		err = db.DeleteUser(toDelete)
		if err != nil {
			return err
		}

		RemoveUsersSessions(toDelete)
		RemoveUsersTokens(toDelete)
	}

	return nil
}

// Delete the sessions owned by a user
func RemoveUsersSessions(user *model.User) {
	cache := database.GetCacheInstance()

	// Find sessions for the user and delete them
	sessions, err := cache.GetSessionsForUser(user.Id)
	if err == nil && sessions != nil {
		for _, session := range sessions {
			cache.DeleteSession(session)
		}
	}
}

// Delete the tokens owned by a user
func RemoveUsersTokens(user *model.User) {
	db := database.GetInstance()

	// Find API tokens for the user and delete them
	tokens, err := db.GetTokensForUser(user.Id)
	if err == nil && tokens != nil {
		for _, token := range tokens {
			db.DeleteToken(token)
		}
	}
}

func updateSpacesSSHKey(user *model.User) {
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
		if space.IsDeployed || space.TemplateId == model.MANUAL_TEMPLATE_ID {
			// Get the agent state
			agentState := agent_server.GetSession(space.Id)
			if agentState == nil {
				// Silently ignore if space is on a different server
				if space.Location == "" || space.Location == server_info.LeafLocation {
					log.Debug().Msgf("Agent state not found for space %s", space.Id)
				}
				continue
			}

			// If agent accepting SSH keys then update
			if agentState.SSHPort > 0 {
				log.Debug().Msgf("Sending SSH public key to agent %s", space.Id)
				if err := agentState.SendUpdateAuthorizedKeys(user.SSHPublicKey, user.GitHubUsername); err != nil {
					log.Debug().Msgf("Failed to send SSH public key to agent: %s", err)
				}
			}
		}
	}

	log.Debug().Msgf("Finished updating agent SSH key for user %s", user.Id)
}

// For disabled users ensure all spaces are stopped, for enabled users update the SSH key on the agents
func UpdateUserSpaces(user *model.User) {
	// If the user is disabled then stop all spaces
	if !user.Active {
		spaces, err := database.GetInstance().GetSpacesForUser(user.Id)
		if err != nil {
			return
		}

		// Get the nomad client
		db := database.GetInstance()
		nomadClient := nomad.NewClient()
		containerClient := docker.NewClient()
		for _, space := range spaces {
			if space.IsDeployed && (space.Location == "" || space.Location == server_info.LeafLocation) {

				// Load the space template
				template, err := db.GetTemplate(space.TemplateId)
				if err != nil {
					log.Debug().Msgf("Failed to get template for space %s: %s", space.Id, err)
					continue
				}

				if template.LocalContainer {
					containerClient.DeleteSpaceJob(space)
				} else {
					nomadClient.DeleteSpaceJob(space)
				}
			}
		}

		// Kill the sessions to logout the user, but leave the tokens there until they expire
		RemoveUsersSessions(user)
	} else {
		// Update the SSH key on the agents
		updateSpacesSSHKey(user)
	}
}
