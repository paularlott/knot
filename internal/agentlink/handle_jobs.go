package agentlink

import (
	"net"

	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/spacejobs"
)

// handleJobsList returns the current jobs snapshot (definitions, schedules,
// run state, history) for the local scheduler.
func handleJobsList(conn net.Conn) {
	snapshot, err := spacejobs.Snapshot()
	if err != nil {
		log.WithError(err).Error("Failed to build jobs snapshot")
		snapshot = &spacejobs.JobsSnapshot{}
	}

	if err := sendMsg(conn, CommandNil, snapshot); err != nil {
		log.WithError(err).Error("Failed to send jobs list response")
	}
}

// handleJobsRun starts a job immediately by name.
func handleJobsRun(conn net.Conn, msg *CommandMsg) {
	var req JobsRunRequest
	if err := msg.Unmarshal(&req); err != nil {
		log.WithError(err).Error("Failed to unmarshal jobs run request")
		return
	}

	response := JobsResponse{Success: true}
	if err := spacejobs.RunJob(req.Name); err != nil {
		response.Success = false
		response.Error = err.Error()
	}

	if err := sendMsg(conn, CommandNil, response); err != nil {
		log.WithError(err).Error("Failed to send jobs run response")
	}
}
