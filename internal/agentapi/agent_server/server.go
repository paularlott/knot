package agent_server

import (
	"crypto/tls"
	"net"
	"sync"
	"time"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/health"
	"github.com/paularlott/knot/internal/methods"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/spaceutil"

	"github.com/paularlott/knot/internal/log"
)

const (
	AGENT_SCHEDULE_INTERVAL       = 1 * time.Minute
	AGENT_LIVENESS_CHECK_INTERVAL = 10 * time.Second
	AGENT_LIVENESS_TIMEOUT        = 30 * time.Second
)

var (
	sessionMutex              = sync.RWMutex{}
	sessions                  = make(map[string]*Session)
	disconnectReconcileMutex  = sync.Mutex{}
	disconnectReconcileActive = make(map[string]bool)
)

type stopListItem struct {
	space   *model.Space
	session *Session
}

func checkStaleSessions() {
	logger := log.WithGroup("agent")
	logger.Info("starting stale session checker")

	go func() {
		ticker := time.NewTicker(AGENT_LIVENESS_CHECK_INTERVAL)
		defer ticker.Stop()

		for range ticker.C {
			now := time.Now().UTC()
			staleSessions := make([]string, 0)

			sessionMutex.RLock()
			for spaceId, session := range sessions {
				if session == nil || session.LastStateAt.IsZero() {
					continue
				}
				if now.Sub(session.LastStateAt) > AGENT_LIVENESS_TIMEOUT {
					staleSessions = append(staleSessions, spaceId)
				}
			}
			sessionMutex.RUnlock()

			if len(staleSessions) == 0 {
				continue
			}

			db := database.GetInstance()
			for _, spaceId := range staleSessions {
				ExpireSession(spaceId)

				space, err := db.GetSpace(spaceId)
				if err != nil || space == nil || space.IsDeleted || !space.IsDeployed {
					continue
				}

				template, err := db.GetTemplate(space.TemplateId)
				if err != nil || template == nil || template.IsManual() {
					if err == nil && template != nil && template.IsManual() {
						if err := spaceutil.MarkSpaceStopped(space); err != nil {
							logger.WithError(err).Error("failed to mark stale manual space stopped", "space_id", space.Id)
						}
					}
					continue
				}

				if template.IsLocalContainer() {
					if nodeIdCfg, err := db.GetCfgValue("node_id"); err == nil && nodeIdCfg != nil && space.NodeId != "" && space.NodeId != nodeIdCfg.Value {
						continue
					}
				}

				if shouldRestartOnAgentLoss(template) {
					logger.Info("stale agent health check failed, restarting space", "space_id", space.Id, "space_name", space.Name)
					if err := service.GetContainerService().RestartSpace(space); err != nil {
						logger.WithError(err).Error("failed to restart stale agent space", "space_id", space.Id)
					}
					continue
				}

				refs, err := spaceutil.ListRunningRuntimeRefs(template, []*model.Space{space})
				if err != nil {
					logger.WithError(err).Error("failed to list runtime refs for stale session", "space_id", space.Id)
					continue
				}

				if spaceutil.RuntimeRefRunning(space, template, refs) {
					logger.Info("stale agent session with live runtime, stopping via provider", "space_id", space.Id, "space_name", space.Name)
					if err := service.GetContainerService().StopSpace(space); err != nil {
						logger.WithError(err).Error("failed to stop stale live runtime", "space_id", space.Id)
					}
					continue
				}

				logger.Info("stale agent session with missing runtime, marking stopped", "space_id", space.Id, "space_name", space.Name)
				if err := spaceutil.MarkSpaceStopped(space); err != nil {
					logger.WithError(err).Error("failed to mark stale missing runtime stopped", "space_id", space.Id)
				}
			}
		}
	}()
}

func queueDisconnectedSpaceReconcile(spaceId string) {
	if spaceId == "" {
		return
	}

	disconnectReconcileMutex.Lock()
	if disconnectReconcileActive[spaceId] {
		disconnectReconcileMutex.Unlock()
		return
	}
	disconnectReconcileActive[spaceId] = true
	disconnectReconcileMutex.Unlock()

	go func() {
		defer func() {
			disconnectReconcileMutex.Lock()
			delete(disconnectReconcileActive, spaceId)
			disconnectReconcileMutex.Unlock()
		}()

		time.Sleep(AGENT_LIVENESS_TIMEOUT)

		if GetSession(spaceId) != nil {
			return
		}

		db := database.GetInstance()
		space, err := db.GetSpace(spaceId)
		if err != nil || space == nil || space.IsDeleted || !space.IsDeployed || space.IsPending {
			return
		}

		template, err := db.GetTemplate(space.TemplateId)
		if err != nil || template == nil {
			return
		}

		if template.IsManual() {
			if err := spaceutil.MarkSpaceStopped(space); err != nil {
				log.WithError(err).Error("failed to mark disconnected manual space stopped", "space_id", space.Id)
			}
			return
		}

		if template.IsLocalContainer() {
			if nodeIdCfg, err := db.GetCfgValue("node_id"); err == nil && nodeIdCfg != nil && space.NodeId != "" && space.NodeId != nodeIdCfg.Value {
				return
			}
		}

		if shouldRestartOnAgentLoss(template) {
			log.Info("disconnected agent health check failed, restarting space", "space_id", space.Id, "space_name", space.Name)
			if err := service.GetContainerService().RestartSpace(space); err != nil {
				log.WithError(err).Error("failed to restart disconnected agent space", "space_id", space.Id)
			}
			return
		}

		refs, err := spaceutil.ListRunningRuntimeRefs(template, []*model.Space{space})
		if err != nil {
			log.WithError(err).Error("failed to list runtime refs for disconnected space", "space_id", space.Id)
			return
		}

		if spaceutil.RuntimeRefRunning(space, template, refs) {
			log.Info("disconnected space still has live runtime, stopping via provider", "space_id", space.Id, "space_name", space.Name)
			if err := service.GetContainerService().StopSpace(space); err != nil {
				log.WithError(err).Error("failed to stop disconnected live runtime", "space_id", space.Id)
			}
			return
		}

		log.Info("disconnected space runtime missing, marking stopped", "space_id", space.Id, "space_name", space.Name)
		if err := spaceutil.MarkSpaceStopped(space); err != nil {
			log.WithError(err).Error("failed to mark disconnected missing runtime stopped", "space_id", space.Id)
		}
	}()
}

// Periodically check to see if the space has a schedule which requires it be stopped
func checkSchedules() {
	logger := log.WithGroup("agent")
	logger.Info("starting schedule checker")

	cfg := config.GetServerConfig()
	go func() {
		ticker := time.NewTicker(AGENT_SCHEDULE_INTERVAL)
		defer ticker.Stop()

		for range ticker.C {
			logger.Debug("checking schedules")

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
				logger.Info("stopping session  due to schedule", "session_id", item.session.Id)
				service.GetContainerService().StopSpace(item.space)
			}
			sessionStopList = nil

			// Look for spaces that need to be started
			spaces, err := db.GetSpaces()
			if err != nil {
				logger.WithError(err).Error("failed to get spaces")
				continue
			}

			for _, space := range spaces {
				if !space.IsDeleted && !space.IsDeployed && !space.IsPending {
					template, err := db.GetTemplate(space.TemplateId)
					if err != nil {
						continue
					}

					if !template.IsManual() && template.ScheduleEnabled && template.AutoStart && template.AllowedBySchedule() {
						logger.Info("starting space  due to schedule", "space_id", space.Id)

						user, err := db.GetUser(space.UserId)
						if err != nil {
							logger.WithError(err).Error("GetUser")
							continue
						}

						if !cfg.LeafNode {
							// Check the users quota has enough compute units
							usage, err := database.GetUserUsage(user.Id, "")
							if err != nil {
								logger.WithError(err).Error("GetUserUsage")
								continue
							}

							userQuota, err := database.GetUserQuota(user)
							if err != nil {
								logger.WithError(err).Error("GetUserQuota")
								continue
							}

							if usage.ComputeUnits+template.ComputeUnits > userQuota.ComputeUnits {
								logger.Warn("user  has insufficient compute units to start space", "username", user.Username, "space_name", space.Name)
								continue
							}
						}

						transport := service.GetTransport()
						unlockToken := transport.LockResource(space.Id)
						if unlockToken == "" {
							logger.Error("failed to lock space")
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

func QueueSpaceReconcile(spaceId string) {
	queueDisconnectedSpaceReconcile(spaceId)
	health.Set(spaceId, false, 0)
}

func shouldRestartOnAgentLoss(template *model.Template) bool {
	return template != nil &&
		template.HealthCheckType == model.HealthCheckAgent &&
		template.HealthCheckAutoRestart &&
		(template.IsLocalContainer() || template.Platform == model.PlatformNomad)
}

func ListenAndServe(listen string, tlsConfig *tls.Config) {
	logger := log.WithGroup("agent")
	service.SetPoolSessionProvider(GetPoolSessionState)

	// Start the session garbage collector & schedule checker
	checkSchedules()
	checkStaleSessions()

	logger.Info("listening for agents on", "listen", listen)

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
			logger.Fatal("Error starting agent listener", "err", err)
		}
		defer listener.Close()

		// Run forever listening for new connections
		for {
			conn, err := listener.Accept()
			if err != nil {
				logger.WithError(err).Error("Error accepting connection")
				continue
			}

			// Start a new goroutine to handle the connection
			go handleAgentConnection(conn)
		}
	}()
}

func removeSession(spaceId string, markUnhealthy bool, queueReconcile bool) {
	var removed bool
	sessionMutex.Lock()
	if session, ok := sessions[spaceId]; ok {
		if session.MuxSession != nil {
			session.MuxSession.Close()
		}
		removed = true
	}
	delete(sessions, spaceId)
	sessionMutex.Unlock()

	if removed {
		methods.DefaultRegistry().UnregisterSpace(spaceId)

		if markUnhealthy || queueReconcile {
			db := database.GetInstance()
			space, err := db.GetSpace(spaceId)
			if err == nil && space != nil {
				if space.IsPending || space.IsDeleting || space.IsDeleted || !space.IsDeployed {
					markUnhealthy = false
					queueReconcile = false
				}
			}
		}

		if markUnhealthy {
			health.Set(spaceId, false, 0)
		} else {
			health.Delete(spaceId)
		}
		if queueReconcile {
			queueDisconnectedSpaceReconcile(spaceId)
		}
	}
}

func RemoveSession(spaceId string) {
	removeSession(spaceId, false, false)
}

func DisconnectSession(spaceId string) {
	removeSession(spaceId, true, true)
}

func ExpireSession(spaceId string) {
	removeSession(spaceId, true, false)
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

func GetPoolSessionState(spaceId string) *service.PoolSessionState {
	session := GetSession(spaceId)
	if session == nil {
		return nil
	}
	return &service.PoolSessionState{
		CPUPercent:       session.CPUPercent,
		MemoryUsedBytes:  session.MemoryUsedBytes,
		MemoryLimitBytes: session.MemoryLimitBytes,
		MethodRPS:        session.MethodRPS,
		HTTPRPS:          session.HTTPRPS,
		TCPRPS:           session.TCPRPS,
	}
}
