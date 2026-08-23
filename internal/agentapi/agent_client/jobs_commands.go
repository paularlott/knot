package agent_client

import (
	"net"

	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/spacejobs"
)

// handleJobsListExecution answers the server's jobs list command with the
// local scheduler's snapshot. Runs in the agent process alongside the
// scheduler, so it reads state directly.
func handleJobsListExecution(stream net.Conn) {
	snapshot, err := spacejobs.Snapshot()
	if err != nil {
		log.WithError(err).Error("jobs: failed to build jobs snapshot")
		snapshot = &spacejobs.JobsSnapshot{}
	}

	if err := msg.WriteMessage(stream, snapshot); err != nil {
		log.WithError(err).Error("jobs list: failed to write response")
	}
}

// handleJobsRunExecution starts a job immediately by name.
func handleJobsRunExecution(stream net.Conn, req msg.JobsRunMessage) {
	response := msg.JobsResponse{Success: true}
	if err := spacejobs.RunJob(req.Name); err != nil {
		response.Success = false
		response.Error = err.Error()
	}

	if err := msg.WriteMessage(stream, &response); err != nil {
		log.WithError(err).Error("jobs run: failed to write response")
	}
}

// handleUpdateJobsExecution replaces the scheduler's job definitions with the
// latest push from the server. Fire-and-forget: no reply is written, the
// stream closes when the handler returns.
func handleUpdateJobsExecution(req msg.UpdateJobsMessage) {
	spacejobs.Update(req.Jobs, req.Enabled)
}
