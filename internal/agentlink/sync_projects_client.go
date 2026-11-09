package agentlink

import (
	"errors"
	"fmt"
	"time"

	"github.com/paularlott/knot/internal/syncproject"
)

// syncProjectTimeout bounds a sync project request: a stop sends the
// project's last changes first.
const syncProjectTimeout = 30 * time.Second

// SyncProjects asks the space's agent, over its command socket, to run sync
// projects: for knot file sync-project and scripts run with knot run-script.
type SyncProjects struct{}

// Start runs a project from its file's content, named name when the
// content gives no name, and returns its name.
func (SyncProjects) Start(content, name string) (string, error) {
	resp, err := syncProjectRequest(CommandSyncProjectStart, SyncProjectStartRequest{Content: content, Name: name})
	if err != nil {
		return "", err
	}
	return resp.Name, nil
}

// Stop stops a project and forgets it.
func (SyncProjects) Stop(name string) error {
	_, err := syncProjectRequest(CommandSyncProjectStop, SyncProjectStopRequest{Name: name})
	return err
}

// List is how the projects the agent runs are doing.
func (SyncProjects) List() ([]syncproject.ProjectStatus, error) {
	resp, err := syncProjectRequest(CommandSyncProjectList, nil)
	if err != nil {
		return nil, err
	}
	return resp.Projects, nil
}

func syncProjectRequest(cmd CommandType, payload interface{}) (*SyncProjectResponse, error) {
	if !IsAgentRunning() {
		return nil, errors.New("the space's agent runs sync projects, and none is running here")
	}
	var resp SyncProjectResponse
	if err := SendWithResponseMsgTimeout(cmd, payload, &resp, syncProjectTimeout); err != nil {
		if err == ErrSpaceLocked {
			return nil, errors.New("this space is locked: its processes cannot manage sync projects")
		}
		return nil, fmt.Errorf("asking the space's agent: %w", err)
	}
	if !resp.Success {
		return nil, errors.New(resp.Error)
	}
	return &resp, nil
}
