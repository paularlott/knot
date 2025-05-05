package agent_server

import (
	"crypto/tls"
	"net"
	"sync"
	"time"

	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/internal/container"
	"github.com/paularlott/knot/internal/container/docker"
	"github.com/paularlott/knot/internal/container/nomad"

	"github.com/rs/zerolog/log"
)

const (
	AGENT_SCHEDULE_INTERVAL = 1 * time.Minute
)

var (
	sessionMutex = sync.RWMutex{}
	sessions     = make(map[string]*Session)
)

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

				sessionMutex.RLock()
				for _, session := range sessions {
					db := database.GetInstance()

					space, err := db.GetSpace(session.Id)
					if err != nil {
						continue
					}

					template, err := db.GetTemplate(space.TemplateId)
					if err != nil {
						continue
					}

					if !template.AllowedBySchedule() {
						log.Info().Msgf("agent: stopping space %s due to schedule", space.Id)

						// Mark the space as pending and save it
						space.IsPending = true
						if err = db.SaveSpace(space, []string{"IsPending"}); err != nil {
							log.Error().Msgf("DeleteSpaceJob: failed to save space %s", err.Error())
							continue
						}

						var containerClient container.ContainerManager
						if template.LocalContainer {
							containerClient = docker.NewClient()
						} else {
							containerClient = nomad.NewClient()
						}

						// Stop the job
						err = containerClient.DeleteSpaceJob(space)
						if err != nil {
							space.IsPending = false
							db.SaveSpace(space, []string{"IsPending"})

							log.Error().Msgf("DeleteSpaceJob: failed to delete space %s", err.Error())
							continue
						}

					}
				}
				sessionMutex.RUnlock()
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
