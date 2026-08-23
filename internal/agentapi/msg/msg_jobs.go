package msg

import (
	"net"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/log"
)

// JobsRunMessage asks the agent to start a job immediately by name. Sent from
// the server to the agent.
type JobsRunMessage struct {
	Name string `json:"name" msgpack:"name"`
}

// UpdateJobsMessage replaces the agent's in-memory job definitions and runner
// state with the space's persisted definitions. Sent from the server to the
// agent on every change; the registration response carries the same data on
// (re)connect.
type UpdateJobsMessage struct {
	Jobs    []model.SpaceJob `json:"jobs" msgpack:"jobs"`
	Enabled bool             `json:"enabled" msgpack:"enabled"`
}

// JobsResponse is the outcome of a run request.
type JobsResponse struct {
	Success bool   `json:"success" msgpack:"success"`
	Error   string `json:"error,omitempty" msgpack:"error,omitempty"`
}

// SendUpdateJobs pushes new job definitions to the agent. Fire-and-forget:
// the agent applies the push without replying, and the definitions are
// re-sent in the registration response if the connection was down.
func SendUpdateJobs(conn net.Conn, jobs []model.SpaceJob, enabled bool) error {
	logger := log.WithGroup("agent")
	if err := WriteCommand(conn, CmdUpdateJobs); err != nil {
		logger.WithError(err).Error("writing update jobs command")
		return err
	}
	if err := WriteMessage(conn, &UpdateJobsMessage{Jobs: jobs, Enabled: enabled}); err != nil {
		logger.WithError(err).Error("writing update jobs message")
		return err
	}
	return nil
}
