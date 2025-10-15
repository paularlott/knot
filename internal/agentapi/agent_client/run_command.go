package agent_client

import (
	"context"
	"net"
	"os/exec"
	"strings"
	"time"

	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/util"

	"github.com/paularlott/knot/internal/log"
)

func handleRunCommandExecution(stream net.Conn, runCmd msg.RunCommandMessage) {
	log.Debug("agent: executing run command", "command", runCmd.Command, "args", runCmd.Args)

	if runCmd.Command == "" && len(runCmd.Args) == 0 {
		response := msg.RunCommandResponse{Success: false, Error: "Empty command"}
		msg.WriteMessage(stream, &response)
		return
	}

	// Always invoke via shell to support pipes/redirection.
	// Combine command and args into a single shell command string
	var parts []string
	if runCmd.Command != "" {
		parts = append(parts, runCmd.Command)
	}
	parts = append(parts, runCmd.Args...)
	shellCmd := strings.Join(parts, " ")

	log.Debug("agent: constructed shell command", "final_shell_command", shellCmd)

	timeout := time.Duration(runCmd.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Find the best available shell (reusing the same logic as the SSH server)
	selectedShell := util.CheckShells("bash")
	if selectedShell == "" {
		response := msg.RunCommandResponse{Success: false, Error: "No valid shell found"}
		msg.WriteMessage(stream, &response)
		return
	}

	log.Debug("agent: using shell", "selected_shell", selectedShell)

	// Use -c flag only (no login shell to avoid profile loading issues)
	cmd := exec.CommandContext(ctx, selectedShell, "-c", shellCmd)
	if runCmd.Workdir != "" {
		cmd.Dir = runCmd.Workdir
	}

	log.Debug("agent: executing shell command", "shell", selectedShell, "shell_command", shellCmd, "workdir", runCmd.Workdir)

	output, err := cmd.CombinedOutput()

	log.Debug("agent: raw command output", "raw_output_bytes", len(output), "raw_output", string(output))
	if err != nil {
		log.WithError(err).Error("agent: command execution error")
	}

	response := msg.RunCommandResponse{Output: output}
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

	log.Debug("agent: run command execution completed", "shell", selectedShell, "command", shellCmd, "output_bytes", len(response.Output), "success", response.Success, "error", response.Error)

	if err := msg.WriteMessage(stream, &response); err != nil {
		log.WithError(err).Error("agent: failed to send run command response")
		return
	}

	log.Debug("agent: response sent successfully")
}
