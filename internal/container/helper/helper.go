package helper

import (
	"fmt"
	"strings"
	"time"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/container"
	"github.com/paularlott/knot/internal/container/apple"
	"github.com/paularlott/knot/internal/container/docker"
	"github.com/paularlott/knot/internal/container/nomad"
	"github.com/paularlott/knot/internal/container/podman"
	"github.com/paularlott/knot/internal/container/runtime"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/spaceutil"
	"github.com/paularlott/knot/internal/sse"

	"github.com/paularlott/knot/internal/log"
)

type Helper struct {
}

func NewContainerHelper() *Helper {
	return &Helper{}
}

func (h *Helper) createClient(platform string) (container.ContainerManager, error) {
	// Map "container" to detected runtime
	if platform == model.PlatformContainer {
		cfg := config.GetServerConfig()
		platform = runtime.DetectLocalContainerRuntime(cfg.LocalContainerRuntimePref)
		if platform == "" {
			return nil, fmt.Errorf("no local container runtime detected")
		}
	}

	switch platform {
	case model.PlatformDocker:
		client := docker.NewClient()
		if client == nil {
			return nil, fmt.Errorf("failed to create docker client")
		}
		return client, nil
	case model.PlatformPodman:
		client := podman.NewClient()
		if client == nil {
			return nil, fmt.Errorf("failed to create podman client")
		}
		return client, nil
	case model.PlatformNomad:
		return nomad.NewClient()
	case model.PlatformApple:
		client := apple.NewClient()
		if client == nil {
			return nil, fmt.Errorf("failed to create apple client")
		}
		return client, nil
	default:
		return nil, fmt.Errorf("unsupported platform: %s", platform)
	}
}

func (h *Helper) CleanupMigratedSpaceArtifacts(space *model.Space, template *model.Template) error {
	if template == nil || !template.IsLocalContainer() {
		return nil
	}

	containerClient, err := h.createClient(template.Platform)
	if err != nil {
		log.WithError(err).Error("CleanupMigratedSpaceArtifacts: failed to create container client")
		return err
	}

	return containerClient.CleanupSpaceArtifacts(space)
}

func (h *Helper) ListRunningSpaceRuntimeRefs(template *model.Template, spaces []*model.Space) (map[string]bool, error) {
	return spaceutil.ListRunningRuntimeRefs(template, spaces)
}

func (h *Helper) CreateVolume(volume *model.Volume) error {
	db := database.GetInstance()
	cfg := config.GetServerConfig()

	variables, err := db.GetTemplateVars()
	if err != nil {
		return err
	}

	vars := model.FilterVars(variables)

	// Mark volume as started
	volume.Zone = cfg.Zone
	volume.Active = true

	containerClient, err := h.createClient(volume.Platform)
	if err != nil {
		log.WithError(err).Error("CreateVolume: failed to create container client")
		return err
	}

	// Create volumes
	err = containerClient.CreateVolume(volume, vars)
	if err != nil {
		return err
	}

	return nil
}

func (h *Helper) DeleteVolume(volume *model.Volume) error {
	db := database.GetInstance()

	variables, err := db.GetTemplateVars()
	if err != nil {
		return err
	}

	vars := model.FilterVars(variables)

	// Record the volume as not deployed
	volume.Zone = ""
	volume.Active = false

	containerClient, err := h.createClient(volume.Platform)
	if err != nil {
		log.WithError(err).Error("DeleteVolume: failed to create container client")
		return err
	}

	// Delete the volume
	err = containerClient.DeleteVolume(volume, vars)
	if err != nil && !strings.Contains(err.Error(), "volume not found") {
		return err
	}

	return nil
}

func (h *Helper) StartSpace(space *model.Space, template *model.Template, user *model.User) error {
	db := database.GetInstance()

	// Mark the space as pending and save it
	space.IsPending = true
	space.UpdatedAt = hlc.Now()
	if err := db.SaveSpace(space, []string{"IsPending", "UpdatedAt"}); err != nil {
		log.WithError(err).Error("StartSpace")
		return err
	}

	if transport := service.GetTransport(); transport != nil {
		transport.GossipSpace(space)
	}
	sse.PublishSpaceChanged(space.Id, space.UserId)

	// Revert the pending status if the deploy fails
	var deployFailed = true
	defer func() {
		if deployFailed {
			// If the deploy failed then revert the space to not pending
			space.IsPending = false
			space.UpdatedAt = hlc.Now()
			db.SaveSpace(space, []string{"IsPending", "UpdatedAt"})
			if transport := service.GetTransport(); transport != nil {
				transport.GossipSpace(space)
			}
			sse.PublishSpaceChanged(space.Id, space.UserId)
		}
	}()

	// Get the variables
	variables, err := db.GetTemplateVars()
	if err != nil {
		log.WithError(err).Error("StartSpace")
		return err
	}

	vars := model.FilterVars(variables)

	containerClient, err := h.createClient(template.Platform)
	if err != nil {
		log.WithError(err).Error("StartSpace: failed to create container client")
		return err
	}

	// Create volumes
	err = containerClient.CreateSpaceVolumes(user, template, space, vars)
	if err != nil {
		log.WithError(err).Error("StartSpace")
		return err
	}

	// Start the job
	err = containerClient.CreateSpaceJob(user, template, space, vars)
	if err != nil {
		log.WithError(err).Error("StartSpace")
		return err
	}

	// Don't revert the space on success
	deployFailed = false

	// Execute startup script if defined (non-blocking)
	go func() {
		// Execute system startup script (no timeout — runs in the background)
		if err := executeSpaceScript(space, template, user, template.StartupScriptId, true, 0); err != nil {
			log.WithError(err).Warn("system startup script failed", "space_id", space.Id)
		}
		// Execute user startup script from space definition
		if space.StartupScriptId != "" {
			if err := executeSpaceScript(space, template, user, space.StartupScriptId, true, 0); err != nil {
				log.WithError(err).Warn("user startup script failed", "space_id", space.Id)
			}
		}
	}()

	return nil
}

func (h *Helper) StopSpace(space *model.Space) error {
	db := database.GetInstance()

	// Get the template
	template, err := db.GetTemplate(space.TemplateId)
	if err != nil {
		log.WithError(err).Error("StopSpace: failed to get template")
		return err
	}

	// Get the user
	user, err := db.GetUser(space.UserId)
	if err != nil {
		log.WithError(err).Error("StopSpace: failed to get user")
		return err
	}

	// Mark the space as pending and save it
	space.IsPending = true
	space.UpdatedAt = hlc.Now()
	if err = db.SaveSpace(space, []string{"IsPending", "UpdatedAt"}); err != nil {
		log.WithError(err).Error("StopSpace: failed to save space")
		return err
	}
	if transport := service.GetTransport(); transport != nil {
		transport.GossipSpace(space)
	}
	sse.PublishSpaceChanged(space.Id, space.UserId)

	containerClient, err := h.createClient(template.Platform)
	if err != nil {
		log.WithError(err).Error("StopSpace: failed to create container client")
		return err
	}

	// Run the shutdown script (bounded by ShutdownScriptTimeout so a hung agent
	// script can't block the stop) while the agent is still alive, then tear down
	// the job.
	if err := executeSpaceScript(space, template, user, template.ShutdownScriptId, false, ShutdownScriptTimeout); err != nil {
		log.WithError(err).Warn("system shutdown script failed", "space_id", space.Id)
	}

	if err := containerClient.DeleteSpaceJob(space, nil); err != nil {
		space.IsPending = false
		space.UpdatedAt = hlc.Now()
		db.SaveSpace(space, []string{"IsPending", "UpdatedAt"})
		if transport := service.GetTransport(); transport != nil {
			transport.GossipSpace(space)
		}
		sse.PublishSpaceChanged(space.Id, space.UserId)

		log.WithError(err).Error("StopSpace: failed to delete space")
		return err
	}

	return nil
}

func (h *Helper) RestartSpace(space *model.Space) error {
	db := database.GetInstance()

	// Get the template
	template, err := db.GetTemplate(space.TemplateId)
	if err != nil {
		log.WithError(err).Error("RestartSpace: failed to get template")
		return err
	}

	// Mark the space as pending and save it
	space.IsPending = true
	space.UpdatedAt = hlc.Now()
	if err = db.SaveSpace(space, []string{"IsPending", "UpdatedAt"}); err != nil {
		log.WithError(err).Error("RestartSpace: failed to save space")
		return err
	}
	if transport := service.GetTransport(); transport != nil {
		transport.GossipSpace(space)
	}
	sse.PublishSpaceChanged(space.Id, space.UserId)

	// Get the user from the space
	user, err := db.GetUser(space.UserId)
	if err != nil {
		log.WithError(err).Error("RestartSpace: failed to get user")
		return err
	}

	containerClient, err := h.createClient(template.Platform)
	if err != nil {
		log.WithError(err).Error("RestartSpace: failed to create container client")
		return err
	}

	// Run the shutdown script (bounded by ShutdownScriptTimeout) while the agent
	// is still alive, then tear down the job; DeleteSpaceJob's callback starts the
	// container again.
	if err := executeSpaceScript(space, template, user, template.ShutdownScriptId, false, ShutdownScriptTimeout); err != nil {
		log.WithError(err).Warn("system shutdown script failed", "space_id", space.Id)
	}

	if err := containerClient.DeleteSpaceJob(space, func() {
		// Start the container again
		h.StartSpace(space, template, user)
	}); err != nil {
		space.IsPending = false
		space.UpdatedAt = hlc.Now()
		db.SaveSpace(space, []string{"IsPending", "UpdatedAt"})
		if transport := service.GetTransport(); transport != nil {
			transport.GossipSpace(space)
		}
		sse.PublishSpaceChanged(space.Id, space.UserId)

		log.WithError(err).Error("RestartSpace: failed to delete space")
		return err
	}

	return nil
}

func (h *Helper) DeleteSpace(space *model.Space) {
	go func() {
		logger := log.WithGroup("server")
		logger.Info("deleting space", "space_id", space.Id)

		db := database.GetInstance()

		template, err := db.GetTemplate(space.TemplateId)
		if err != nil {
			logger.WithError(err).Error("load template")

			space.IsDeleting = false
			space.UpdatedAt = hlc.Now()
			db.SaveSpace(space, []string{"IsDeleting", "UpdatedAt"})
			if transport := service.GetTransport(); transport != nil {
				transport.GossipSpace(space)
			}
			sse.PublishSpaceChanged(space.Id, space.UserId)
			return
		}

		// If not a manual space then we have to do additional checks and clean up
		if !template.IsManual() {
			containerClient, err := h.createClient(template.Platform)
			if err != nil {
				logger.WithError(err).Error("failed to create container client")

				space.IsDeleting = false
				space.UpdatedAt = hlc.Now()
				db.SaveSpace(space, []string{"IsDeleting", "UpdatedAt"})
				if transport := service.GetTransport(); transport != nil {
					transport.GossipSpace(space)
				}
				sse.PublishSpaceChanged(space.Id, space.UserId)
				return
			}

			// If the space is deployed, stop the job
			if space.IsDeployed {
				// Get user for script execution
				user, err := db.GetUser(space.UserId)
				if err != nil {
					logger.WithError(err).Warn("failed to get user for shutdown scripts")
				} else {
					// Execute shutdown script (bounded by ShutdownScriptTimeout)
					if err := executeSpaceScript(space, template, user, template.ShutdownScriptId, false, ShutdownScriptTimeout); err != nil {
						logger.WithError(err).Warn("system shutdown script failed", "space_id", space.Id)
					}
				}

				err = containerClient.DeleteSpaceJob(space, nil)
				if err != nil {
					logger.WithError(err).Error("delete space job")
					space.IsDeleting = false
					space.UpdatedAt = hlc.Now()
					db.SaveSpace(space, []string{"IsDeleting", "UpdatedAt"})
					if transport := service.GetTransport(); transport != nil {
						transport.GossipSpace(space)
					}
					sse.PublishSpaceChanged(space.Id, space.UserId)
					return
				}
			}

			// Delete volumes, log errors but don't fail the space deletion
			err = containerClient.DeleteSpaceVolumes(space)
			if err != nil {
				logger.WithError(err).Error("delete space volumes")
			}
		}

		// Delete the space
		oldSpace := *space
		space.IsDeleted = true
		space.Name = space.Id
		space.DependsOn = []string{}
		space.UpdatedAt = hlc.Now()
		err = db.SaveSpace(space, []string{"IsDeleted", "UpdatedAt", "Name", "DependsOn"})
		if err != nil {
			logger.WithError(err).Error("delete space")
			return
		}

		service.CheckSpaceLifecycleEvents(&oldSpace, space)

		if err := service.GetSpaceService().RemoveDependencyReferences(space.Id, space.UserId); err != nil {
			logger.WithError(err).Error("delete space dependencies")
			return
		}

		if transport := service.GetTransport(); transport != nil {
			transport.GossipSpace(space)
		}
		sse.PublishSpaceDeleted(space.Id, space.UserId)

		// Delete the agent state if present
		agent_server.RemoveSession(space.Id)

		logger.Info("deleted space", "space_id", space.Id)
	}()
}

// Clean up spaces in broken states during boot.
// Runs before joining the cluster so only stops orphaned runtimes and
// handles definitive state transitions (IsDeleting, IsPending stops).
// Does NOT start stopped spaces — monitoring will catch those after sync.
func (h *Helper) CleanupOnBoot() {
	logger := log.WithGroup("server")
	logger.Info("cleaning spaces...")

	db := database.GetInstance()
	cfg := config.GetServerConfig()

	var localNodeId string
	if nodeIdCfg, err := db.GetCfgValue("node_id"); err == nil && nodeIdCfg != nil {
		localNodeId = nodeIdCfg.Value
	}

	spaces, err := db.GetSpaces()
	if err != nil {
		logger.WithError(err).Fatal("failed to get spaces")
		return
	}

	templateCache := make(map[string]*model.Template)
	runtimeRefCache := make(map[string]map[string]bool)

	for _, space := range spaces {
		if space.IsDeleted || space.Zone != cfg.Zone {
			continue
		}

		template, ok := templateCache[space.TemplateId]
		if !ok {
			template, err = db.GetTemplate(space.TemplateId)
			if err != nil {
				logger.WithError(err).Error("failed to get template from space")
				continue
			}
			templateCache[space.TemplateId] = template
		}

		if space.IsDeleting {
			logger.Info("found space pending delete, restarting delete...", "space_name", space.Name)
			h.DeleteSpace(space)
			continue
		}

		if template.IsManual() {
			continue
		}

		if template.IsLocalContainer() {
			if space.NodeId != "" && space.NodeId != localNodeId {
				continue
			}

			resolved := template.Platform
			if resolved == model.PlatformContainer {
				resolved = runtime.DetectLocalContainerRuntime(cfg.LocalContainerRuntimePref)
			}
			if resolved == "" {
				continue
			}
			available := runtime.DetectAllAvailableRuntimes(cfg.LocalContainerRuntimePref)
			found := false
			for _, rt := range available {
				if rt == resolved {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		containerClient, err := h.createClient(template.Platform)
		if err != nil {
			logger.WithError(err).Error("failed to create container client for boot cleanup", "space_name", space.Name)
			continue
		}

		runtimeKey := template.Platform
		if template.Platform == model.PlatformContainer {
			runtimeKey = runtime.DetectLocalContainerRuntime(cfg.LocalContainerRuntimePref)
		}
		if template.Platform == model.PlatformNomad {
			runtimeKey = template.Platform + ":" + spaceutil.NormalizeNomadNamespace(space.NomadNamespace)
		}

		refs, ok := runtimeRefCache[runtimeKey]
		if !ok {
			refs, err = h.ListRunningSpaceRuntimeRefs(template, []*model.Space{space})
			if err != nil {
				logger.WithError(err).Error("failed to list running runtimes for boot cleanup", "space_name", space.Name)
				continue
			}
			runtimeRefCache[runtimeKey] = refs
		}

		running := spaceutil.RuntimeRefRunning(space, template, refs)

		if running && !space.IsDeployed {
			logger.Info("found orphaned runtime for stopped space, stopping runtime...", "space_name", space.Name)
			if err := containerClient.StopSpaceRuntime(space); err != nil {
				logger.WithError(err).Error("failed to stop orphaned runtime for space", "space_name", space.Name)
			}
			continue
		}

		if space.IsPending && space.IsDeployed && running {
			logger.Info("found space  pending stop with live runtime, stopping...", "space_name", space.Name)
			h.StopSpace(space)
			continue
		}

		if space.IsDeployed && !space.IsPending {
			logger.Info("queuing reconcile for deployed space", "space_name", space.Name)
			agent_server.QueueSpaceReconcile(space.Id)
		}
	}

	logger.Info("finished cleaning spaces...")
}

// ShutdownScriptTimeout bounds how long StopSpace/RestartSpace wait for a
// space's shutdown script to finish before proceeding with teardown, so a hung
// agent script cannot block the stop indefinitely. Startup scripts pass a zero
// timeout (they run in the background and may take as long as they need).
var ShutdownScriptTimeout = 60 * time.Second

func executeSpaceScript(space *model.Space, template *model.Template, user *model.User, scriptId string, waitForAgent bool, timeout time.Duration) error {
	if scriptId == "" || template.IsManual() {
		return nil
	}

	db := database.GetInstance()
	script, err := db.GetScript(scriptId)
	if err != nil {
		log.WithError(err).Error("failed to get script", "script_id", scriptId, "space_id", space.Id)
		return err
	}

	if script == nil || script.IsDeleted || !script.Active {
		log.Debug("script not found or inactive", "script_id", scriptId, "space_id", space.Id)
		return nil
	}

	return executeScript(space, script, waitForAgent, timeout)
}

// executeScript sends a script to the space's agent and waits for the result.
// A timeout of 0 means wait indefinitely; a positive timeout returns an error
// if the agent does not respond in time (the agent goroutine's response is
// absorbed by the buffered response channel, so no goroutine leaks).
func executeScript(space *model.Space, script *model.Script, waitForAgent bool, timeout time.Duration) error {
	var session *agent_server.Session
	if waitForAgent {
		for i := 0; i < 60; i++ {
			session = agent_server.GetSession(space.Id)
			if session != nil {
				break
			}
			time.Sleep(5 * time.Second)
		}
	} else {
		session = agent_server.GetSession(space.Id)
	}

	if session == nil {
		log.Warn("agent not connected, skipping script", "space_id", space.Id)
		return nil
	}

	log.Trace("executing script", "script_id", script.Id, "space_id", space.Id)

	// Startup and system scripts run without timeout
	execMsg := &msg.ExecuteScriptMessage{
		Content:      script.Content,
		Arguments:    []string{},
		IsSystemCall: true,
	}

	respChan, err := session.SendExecuteScript(execMsg)
	if err != nil {
		log.WithError(err).Error("failed to send script to agent", "script_id", script.Id, "space_id", space.Id)
		return err
	}

	var resp *msg.ExecuteScriptResponse
	if timeout > 0 {
		select {
		case resp = <-respChan:
		case <-time.After(timeout):
			log.Warn("script execution timed out", "script_id", script.Id, "space_id", space.Id, "timeout", timeout)
			return fmt.Errorf("script execution timed out after %s", timeout)
		}
	} else {
		resp = <-respChan
	}

	if !resp.Success {
		err := fmt.Errorf("%s", resp.Error)
		log.WithError(err).Error("script execution failed", "script_id", script.Id, "space_id", space.Id)
		return err
	}

	log.Trace("script completed", "script_id", script.Id, "space_id", space.Id)
	return nil
}
