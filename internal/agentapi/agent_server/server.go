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

	"github.com/paularlott/knot/internal/log"
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
	log.Info("agent: starting schedule checker")

	cfg := config.GetServerConfig()
	go func() {
		ticker := time.NewTicker(AGENT_SCHEDULE_INTERVAL)
		defer ticker.Stop()

		for range ticker.C {
			log.Debug("agent: checking schedules")

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
				log.Info("agent: stopping session  due to schedule", "session_id", item.session.Id)
				service.GetContainerService().StopSpace(item.space)
			}
			sessionStopList = nil

			// Look for spaces that need to be started
			spaces, err := db.GetSpaces()
			if err != nil {
				log.WithError(err).Error("agent: failed to get spaces:")
				continue
			}

			for _, space := range spaces {
				if !space.IsDeleted && !space.IsDeployed && !space.IsPending {
					template, err := db.GetTemplate(space.TemplateId)
					if err != nil {
						continue
					}

					if !template.IsManual() && template.ScheduleEnabled && template.AutoStart && template.AllowedBySchedule() {
						log.Info("agent: starting space  due to schedule", "space_id", space.Id)

						user, err := db.GetUser(space.UserId)
						if err != nil {
							log.WithError(err).Error("agent: GetUser")
							continue
						}

						if !cfg.LeafNode {
							// Check the users quota has enough compute units
							usage, err := database.GetUserUsage(user.Id, "")
							if err != nil {
								log.WithError(err).Error("agent: GetUserUsage")
								continue
							}

							userQuota, err := database.GetUserQuota(user)
							if err != nil {
								log.WithError(err).Error("agent: GetUserQuota")
								continue
							}

							if usage.ComputeUnits+template.ComputeUnits > userQuota.ComputeUnits {
								log.Warn("agent: user  has insufficient compute units to start space", "username", user.Username, "space_name", space.Name)
								continue
							}
						}

						transport := service.GetTransport()
						unlockToken := transport.LockResource(space.Id)
						if unlockToken == "" {
							log.Error("checkSchedules: failed to lock space")
							continue
						}
						service.GetContainerService().StartSpace(space, template, user)
						transport.UnlockResource(space.Id, unlockToken)
					}
				}
			}
		}
	}()
}

func ListenAndServe(listen string, tlsConfig *tls.Config) {

	// Start the session garbage collector & schedule checker
	checkSchedules()

	log.Info("server: listening for agents on:", "listen", listen)

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
			log.Fatal("Error starting agent listener:", "err", err)
		}
		defer listener.Close()

		// Run forever listening for new connections
		for {
			conn, err := listener.Accept()
			if err != nil {
				log.WithError(err).Error("Error accepting connection:")
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
