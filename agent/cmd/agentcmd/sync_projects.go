package agentcmd

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"

	"github.com/paularlott/knot/apiclient"
	command_files "github.com/paularlott/knot/command/files"
	"github.com/paularlott/knot/internal/agentapi/agent_client"
	"github.com/paularlott/knot/internal/agentlink"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/syncproject"
)

// startSyncProjects runs the space's sync projects: the template's, as each
// registration gives it, and those started with knot file sync-project
// start, kept in ~/.knot/sync-projects and started again here. They sync as
// the space's processes do, with the token handed to them.
func startSyncProjects(ctx context.Context, agentClient *agent_client.AgentClient) *command_files.ProjectManager {
	dir := ".knot/sync-projects"
	if home, err := os.UserHomeDir(); err == nil {
		dir = filepath.Join(home, dir)
	}
	client := func() (*apiclient.ApiClient, error) {
		server, token := agentClient.GetServerURL(), agentClient.GetAgentCliToken()
		if server == "" || token == "" {
			return nil, errors.New("the agent has not registered with the server yet")
		}
		return apiclient.NewClient(server, token, true)
	}
	m := command_files.NewProjectManager(ctx, dir, client, agent_client.NewAgentClientLogger(agentClient, "knot_sync"))

	agentlink.RegisterHandler(agentlink.CommandSyncProjectStart, func(conn net.Conn, msg *agentlink.CommandMsg) {
		var req agentlink.SyncProjectStartRequest
		resp := agentlink.SyncProjectResponse{}
		if err := msg.Unmarshal(&req); err != nil {
			resp.Error = err.Error()
		} else if name, err := m.Start(req.Content, req.Name); err != nil {
			resp.Error = err.Error()
		} else {
			resp.Success, resp.Name = true, name
		}
		reply(conn, resp)
	})
	agentlink.RegisterHandler(agentlink.CommandSyncProjectStop, func(conn net.Conn, msg *agentlink.CommandMsg) {
		var req agentlink.SyncProjectStopRequest
		resp := agentlink.SyncProjectResponse{}
		if err := msg.Unmarshal(&req); err != nil {
			resp.Error = err.Error()
		} else if err := m.Stop(req.Name); err != nil {
			resp.Error = err.Error()
		} else {
			resp.Success = true
		}
		reply(conn, resp)
	})
	agentlink.RegisterHandler(agentlink.CommandSyncProjectList, func(conn net.Conn, msg *agentlink.CommandMsg) {
		reply(conn, agentlink.SyncProjectResponse{Success: true, Projects: m.List()})
	})

	agentClient.OnTemplateSyncProject(m.SetTemplate)
	agentClient.OnSyncFlush(m.Flush)
	m.Resume()
	return m
}

// syncProjectAgent is the agent's sync projects for the scripts it runs,
// through knot.files.sync.
type syncProjectAgent struct{ m *command_files.ProjectManager }

func (a syncProjectAgent) Start(content, name string) (string, error) {
	return a.m.Start(content, name)
}
func (a syncProjectAgent) Stop(name string) error                     { return a.m.Stop(name) }
func (a syncProjectAgent) List() ([]syncproject.ProjectStatus, error) { return a.m.List(), nil }

func reply(conn net.Conn, resp agentlink.SyncProjectResponse) {
	if err := agentlink.Reply(conn, resp); err != nil {
		log.WithError(err).Error("failed to answer a sync project request")
	}
}
