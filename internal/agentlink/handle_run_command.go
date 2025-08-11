package agentlink

import (
	"context"
	"net"
	"os/exec"
	"strings"
	"time"

	"github.com/paularlott/knot/internal/config"
	"github.com/rs/zerolog/log"
)

func handleRunCommand(conn net.Conn, msg *CommandMsg) {
	var request RunCommandRequest
	var response RunCommandResponse

	err := msg.Unmarshal(&request)
	if err != nil {
		log.Error().Err(err).Msg("agent: Failed to unmarshal run command request")
		response.Success = false
		response.Error = "Failed to parse request"
		sendMsg(conn, CommandRunCommand, response)
		return
	}

	// Check if run commands are disabled
	cfg := config.GetAgentConfig()
	if cfg != nil && cfg.DisableRunCommand {
		log.Info().Str("command", request.Command).Msg("agent: Run command disabled, ignoring request")
		response.Success = false
		response.Error = "Run commands are disabled on this agent"
		sendMsg(conn, CommandRunCommand, response)
		return
	}

	log.Info().Str("command", request.Command).Msg("agent: Executing command")

	// Parse the command and arguments
	parts := strings.Fields(request.Command)
	if len(parts) == 0 {
		response.Success = false
		response.Error = "Empty command"
		sendMsg(conn, CommandRunCommand, response)
		return
	}

	// Create context with timeout
	timeout := time.Duration(request.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Create the command
	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)

	// Set working directory if specified
	if request.Workdir != "" {
		cmd.Dir = request.Workdir
	}

	// Execute the command and capture output
	output, err := cmd.CombinedOutput()
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

	// Send the response first
	sendMsg(conn, CommandRunCommand, response)

	// If successful, send the output
	if response.Success && len(output) > 0 {
		// Send output in chunks to avoid overwhelming the connection
		chunkSize := 4096
		for i := 0; i < len(output); i += chunkSize {
			end := i + chunkSize
			if end > len(output) {
				end = len(output)
			}

			chunk := output[i:end]
			_, err := conn.Write(chunk)
			if err != nil {
				log.Error().Err(err).Msg("agent: Failed to send command output")
				break
			}
		}
	}

	log.Info().Bool("success", response.Success).Str("command", request.Command).Msg("agent: Command execution completed")
}
