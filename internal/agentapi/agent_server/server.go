package agent_server

import (
	"crypto/tls"
	"net"
	"sync"
	"time"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"

	"github.com/rs/zerolog/log"
)

const (
	AGENT_SCHEDULE_INTERVAL = 1 * time.Minute
)

var (
	sessionMutex     = sync.RWMutex{}
	sessions         = make(map[string]*Session)
	createTokenMutex = sync.Mutex{}
)

type stopListItem struct {
	space   *model.Space
	session *Session
}

// Periodically check to see if the space has a schedule which requires it be stopped
func checkSchedules() {
	log.Info().Msg("agent: starting schedule checker")

	go func() {
		ticker := time.NewTicker(AGENT_SCHEDULE_INTERVAL)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				log.Debug().Msg("agent: checking schedules")

				db := database.GetInstance()

				sessionStopList := make([]*stopListItem, 0)
				sessionMutex.RLock()
				for _, session := range sessions {
					space, err := db.GetSpace(session.Id)
					if err != nil {
						continue
					}

					template, err := db.GetTemplate(space.TemplateId)
					if err != nil {
						continue
					}

					if !template.AllowedBySchedule() || space.MaxUptimeReached(template) {
						sessionStopList = append(sessionStopList, &stopListItem{
							space:   space,
							session: session,
						})
					}
				}
				sessionMutex.RUnlock()

				// Stop sessions that need to be stopped
				for _, item := range sessionStopList {
					log.Info().Msgf("agent: stopping session %s due to schedule", item.session.Id)
					service.GetContainerService().StopSpace(item.space)
				}
				sessionStopList = nil

				// Look for spaces that need to be started
				spaces, err := db.GetSpaces()
				if err != nil {
					log.Error().Msgf("agent: failed to get spaces: %v", err)
					continue
				}

				for _, space := range spaces {
					if !space.IsDeleted && !space.IsDeployed && !space.IsPending {
						template, err := db.GetTemplate(space.TemplateId)
						if err != nil {
							continue
						}

						if !template.IsManual && template.ScheduleEnabled && template.AutoStart && template.AllowedBySchedule() {
							log.Info().Msgf("agent: starting space %s due to schedule", space.Id)

							user, err := db.GetUser(space.UserId)
							if err != nil {
								log.Error().Err(err).Msgf("agent: GetUser")
								continue
							}

							if !config.LeafNode {
								// Check the users quota has enough compute units
								usage, err := database.GetUserUsage(user.Id, "")
								if err != nil {
									log.Error().Err(err).Msgf("agent: GetUserUsage")
									continue
								}

								userQuota, err := database.GetUserQuota(user)
								if err != nil {
									log.Error().Err(err).Msgf("agent: GetUserQuota")
									continue
								}

								if usage.ComputeUnits+template.ComputeUnits > userQuota.ComputeUnits {
									log.Warn().Msgf("agent: user %s has insufficient compute units to start space %s", user.Username, space.Name)
									continue
								}
							}

							service.GetContainerService().StartSpace(space, template, user)
						}
					}
				}
			}
		}
	}()
}

func ListenAndServe(listen string, tlsConfig *tls.Config) {

	// Start the session garbage collector & schedule checker
	checkSchedules()

	log.Info().Msgf("server: listening for agents on: %s", listen)

	go func() {

		// Open the agent listener
		var listener net.Listener
		var err error

		if tlsConfig == nil {
			listener, err = net.Listen("tcp", listen)
		} else {
			listener, err = tls.Listen("tcp", listen, tlsConfig)
		}
		if err != nil {
			log.Fatal().Msgf("Error starting agent listener: %v", err)
		}
		defer listener.Close()

		// Run forever listening for new connections
		for {
			conn, err := listener.Accept()
			if err != nil {
				log.Error().Msgf("Error accepting connection: %v", err)
				continue
			}

			// Start a new goroutine to handle the connection
			go handleAgentConnection(conn)
		}
	}()
}

// removeSession removes a session associated with the given spaceId.
// It locks the sessionMutex to ensure thread safety, checks if a session
// exists for the provided spaceId, and if so, closes the MuxSession if it is not nil.
// Finally, it deletes the session from the sessions map and unlocks the sessionMutex.
//
// Parameters:
//   - spaceId: The identifier for the space whose session is to be removed.
func RemoveSession(spaceId string) {
	sessionMutex.Lock()
	if session, ok := sessions[spaceId]; ok {
		if session.MuxSession != nil {
			session.MuxSession.Close()
		}
	}
	delete(sessions, spaceId)
	sessionMutex.Unlock()
}

// GetSession retrieves the agent session associated with the given spaceId.
// If an agent session is found for the provided spaceId,
// it returns the session; otherwise, it returns nil.
func GetSession(spaceId string) *Session {
	sessionMutex.RLock()
	defer sessionMutex.RUnlock()

	if session, ok := sessions[spaceId]; ok {
		return session
	}

	return nil
}
