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
		log.WithError(err).Error("jobs: failed to build snapshot")
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

// handleJobsSetEnabledExecution starts or stops the job runner.
func handleJobsSetEnabledExecution(stream net.Conn, req msg.JobsSetEnabledMessage) {
	response := msg.JobsResponse{Success: true}
	if err := spacejobs.SetEnabled(req.Enabled); err != nil {
		response.Success = false
		response.Error = err.Error()
	}

	if err := msg.WriteMessage(stream, &response); err != nil {
		log.WithError(err).Error("jobs set-enabled: failed to write response")
	}
}
