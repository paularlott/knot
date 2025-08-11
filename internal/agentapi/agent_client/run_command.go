package agent_client

import (
	"context"
	"net"
	"os/exec"
	"strings"
	"time"

	"github.com/paularlott/knot/internal/agentapi/msg"

	"github.com/rs/zerolog/log"
)

func handleRunCommandExecution(stream net.Conn, runCmd msg.RunCommandMessage) {
	log.Info().Str("command", runCmd.Command).Msg("agent: executing run command")

	// Parse the command and arguments
	parts := strings.Fields(runCmd.Command)
	if len(parts) == 0 {
		response := msg.RunCommandResponse{
			Success: false,
			Error:   "Empty command",
		}
		msg.WriteMessage(stream, &response)
		return
	}

	// Create context with timeout
	timeout := time.Duration(runCmd.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Create the command
	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)

	// Set working directory if specified
	if runCmd.Workdir != "" {
		cmd.Dir = runCmd.Workdir
	}

	// Execute the command and capture output
	output, err := cmd.CombinedOutput()

	response := msg.RunCommandResponse{
		Output: output,
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			response.Success = false
			response.Error = "Command timed out"
		} else {
			response.Success = false
			response.Error = err.Error()
		}
	} else {
		response.Success = true
	}

	// Send the response
	if err := msg.WriteMessage(stream, &response); err != nil {
		log.Error().Err(err).Msg("agent: failed to send run command response")
		return
	}

	log.Info().Bool("success", response.Success).Str("command", runCmd.Command).Msg("agent: run command execution completed")
}
