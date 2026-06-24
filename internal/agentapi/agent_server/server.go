package agent_server

import (
	"crypto/tls"
	"net"
	"sync"
	"time"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/health"
	"github.com/paularlott/knot/internal/methods"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/spaceutil"
	"github.com/paularlott/knot/internal/sse"

	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/logger"
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
	agentLossMutex            = sync.Mutex{}
	agentLossFailures         = make(map[string]uint32)
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

			for _, spaceId := range staleSessions {
				ExpireSession(spaceId)

				if result := reconcileAgentLoss(spaceId, "stale", logger); result.retry {
					queueDisconnectedSpaceReconcileAfter(spaceId, result.retryAfter)
				}
			}
		}
	}()
}

func queueDisconnectedSpaceReconcile(spaceId string) {
	queueDisconnectedSpaceReconcileAfter(spaceId, AGENT_LIVENESS_TIMEOUT)
}

func queueDisconnectedSpaceReconcileAfter(spaceId string, initialDelay time.Duration) {
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

		if initialDelay <= 0 {
			initialDelay = AGENT_LIVENESS_TIMEOUT
		}
		time.Sleep(initialDelay)

		for {
			if GetSession(spaceId) != nil {
				return
			}

			result := reconcileAgentLoss(spaceId, "disconnected", log.WithGroup("agent"))
			if !result.retry {
				return
			}
			if result.retryAfter <= 0 {
				result.retryAfter = AGENT_LIVENESS_TIMEOUT
			}
			time.Sleep(result.retryAfter)
		}
	}()
}

type agentLossReconcileResult struct {
	retry      bool
	retryAfter time.Duration
}

func reconcileAgentLoss(spaceId, reason string, logger logger.Logger) agentLossReconcileResult {
	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil || space == nil || space.IsDeleted || !space.IsDeployed || space.IsPending {
		return agentLossReconcileResult{}
	}

	if space.Zone != "" && space.Zone != config.GetServerConfig().Zone {
		return agentLossReconcileResult{}
	}

	template, err := db.GetTemplate(space.TemplateId)
	if err != nil || template == nil {
		return agentLossReconcileResult{}
	}

	if template.IsManual() {
		if err := spaceutil.MarkSpaceStopped(space); err != nil {
			logger.WithError(err).Error("failed to mark agent-lost manual space stopped", "space_id", space.Id)
		}
		return agentLossReconcileResult{}
	}

	if template.IsLocalContainer() {
		if nodeIdCfg, err := db.GetCfgValue("node_id"); err == nil && nodeIdCfg != nil && space.NodeId != "" && space.NodeId != nodeIdCfg.Value {
			return agentLossReconcileResult{}
		}
	}

	failures := recordAgentLossFailure(space.Id)
	health.Set(space.Id, false, failures)

	refs, err := spaceutil.ListRunningRuntimeRefs(template, []*model.Space{space})
	if err != nil {
		logger.WithError(err).Error("failed to list runtime refs for agent-lost space", "space_id", space.Id)
		return agentLossReconcileResult{retry: shouldRestartOnAgentLoss(template), retryAfter: agentLossCheckInterval(template)}
	}

	runtimeRunning := spaceutil.RuntimeRefRunning(space, template, refs)
	if !runtimeRunning {
		if shouldRestartOnAgentLoss(template) {
			logger.Info("agent-lost space runtime missing, restarting space", "space_id", space.Id, "space_name", space.Name, "reason", reason, "failures", failures)
			clearAgentLossFailures(space.Id)
			if err := restartAgentLostSpace(space, template, false); err != nil {
				logger.WithError(err).Error("failed to restart agent-lost missing runtime", "space_id", space.Id)
			}
			return agentLossReconcileResult{}
		}

		logger.Warn("agent session lost and runtime is missing, marking unhealthy", "space_id", space.Id, "space_name", space.Name, "reason", reason, "failures", failures)
		return agentLossReconcileResult{}
	}

	if shouldRestartOnAgentLoss(template) && failures >= agentLossMaxFailures(template) {
		logger.Info("agent health check failure threshold reached, restarting space", "space_id", space.Id, "space_name", space.Name, "reason", reason, "failures", failures)
		clearAgentLossFailures(space.Id)
		if err := restartAgentLostSpace(space, template, true); err != nil {
			logger.WithError(err).Error("failed to restart agent-lost space", "space_id", space.Id)
		}
		return agentLossReconcileResult{}
	}

	logger.Warn("agent session lost, marking unhealthy", "space_id", space.Id, "space_name", space.Name, "reason", reason, "failures", failures, "runtime_running", runtimeRunning, "auto_restart", shouldRestartOnAgentLoss(template))
	return agentLossReconcileResult{
		retry:      shouldRestartOnAgentLoss(template),
		retryAfter: agentLossCheckInterval(template),
	}
}

func restartAgentLostSpace(space *model.Space, template *model.Template, runtimeRunning bool) error {
	if runtimeRunning {
		return service.GetContainerService().RestartSpace(space)
	}

	db := database.GetInstance()
	user, err := db.GetUser(space.UserId)
	if err != nil {
		return err
	}

	space.IsPending = false
	space.IsDeployed = false
	space.UpdatedAt = hlc.Now()
	if err := db.SaveSpace(space, []string{"IsPending", "IsDeployed", "UpdatedAt"}); err != nil {
		return err
	}
	if transport := service.GetTransport(); transport != nil {
		transport.GossipSpace(space)
	}
	sse.PublishSpaceChanged(space.Id, space.UserId)

	return service.GetContainerService().StartSpace(space, template, user)
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
		template.HealthCheckAutoRestart &&
		template.HealthCheckType != "" &&
		template.HealthCheckType != model.HealthCheckNone &&
		(template.IsLocalContainer() || template.Platform == model.PlatformNomad)
}

func agentLossMaxFailures(template *model.Template) uint32 {
	if template == nil || template.HealthCheckMaxFailures == 0 {
		return 3
	}
	return template.HealthCheckMaxFailures
}

func agentLossCheckInterval(template *model.Template) time.Duration {
	if template == nil || template.HealthCheckInterval == 0 {
		return 30 * time.Second
	}
	return time.Duration(template.HealthCheckInterval) * time.Second
}

func recordAgentLossFailure(spaceId string) uint32 {
	agentLossMutex.Lock()
	defer agentLossMutex.Unlock()

	agentLossFailures[spaceId]++
	return agentLossFailures[spaceId]
}

func clearAgentLossFailures(spaceId string) {
	agentLossMutex.Lock()
	defer agentLossMutex.Unlock()

	delete(agentLossFailures, spaceId)
}

func ListenAndServe(listen string, tlsConfig *tls.Config) {
	logger := log.WithGroup("agent")
	service.SetPoolSessionProvider(GetPoolSessionState)
	service.SetAgentHealthConfigUpdater(updateAgentHealthConfigForTemplate)

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

func updateAgentHealthConfigForTemplate(template *model.Template) {
	if template == nil {
		return
	}

	db := database.GetInstance()
	spaces, err := db.GetSpacesByTemplateId(template.Id)
	if err != nil {
		log.WithError(err).Error("failed to load spaces for health config update", "template_id", template.Id)
		return
	}

	config := &msg.HealthConfig{
		HealthCheckType:          template.HealthCheckType,
		HealthCheckConfig:        template.HealthCheckConfig,
		HealthCheckSkipSSLVerify: template.HealthCheckSkipSSLVerify,
		HealthCheckTimeout:       template.HealthCheckTimeout,
		HealthCheckInterval:      template.HealthCheckInterval,
		HealthCheckMaxFailures:   template.HealthCheckMaxFailures,
		HealthCheckAutoRestart:   template.HealthCheckAutoRestart,
	}

	logger := log.WithGroup("agent")
	for _, space := range spaces {
		if space == nil || space.IsDeleted || !space.IsDeployed || space.IsPending {
			continue
		}

		session := GetSession(space.Id)
		if session == nil {
			continue
		}

		if err := session.SendUpdateHealthConfig(config); err != nil {
			logger.WithError(err).Error("failed to update live agent health config", "space_id", space.Id, "space_name", space.Name)
			continue
		}

		clearAgentLossFailures(space.Id)
		if template.HealthCheckType == "" || template.HealthCheckType == model.HealthCheckNone || template.HealthCheckType == model.HealthCheckAgent {
			health.Set(space.Id, true, 0)
			sse.PublishSpaceChanged(space.Id, space.UserId)
		}
	}
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
