package agent_client

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/util"

	"github.com/paularlott/knot/internal/log"
)

func handleRunCommandExecution(stream net.Conn, runCmd msg.RunCommandMessage) {
	log.Debug("executing run command", "command", runCmd.Command, "args", runCmd.Args)

	if runCmd.Command == "" && len(runCmd.Args) == 0 {
		response := msg.RunCommandResponse{Success: false, Error: "Empty command"}
		msg.WriteMessage(stream, &response)
		return
	}

	shellCmd := buildShellCommand(runCmd.Command, runCmd.Args)

	log.Debug("constructed shell command", "final_shell_command", shellCmd)

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

	log.Debug("using shell", "selected_shell", selectedShell)

	// Use -c flag only (no login shell to avoid profile loading issues)
	cmd := exec.CommandContext(ctx, selectedShell, "-c", shellCmd)
	if runCmd.Workdir != "" {
		cmd.Dir = runCmd.Workdir
	}

	log.Debug("executing shell command", "shell", selectedShell, "shell_command", shellCmd, "workdir", runCmd.Workdir)

	var combined, stdout, stderr bytes.Buffer
	combinedWriter := &lockedWriter{w: &combined}
	cmd.Stdout = io.MultiWriter(combinedWriter, &stdout)
	cmd.Stderr = io.MultiWriter(combinedWriter, &stderr)
	err := cmd.Run()
	output := combined.Bytes()

	log.Debug("raw command output", "raw_output_bytes", len(output), "raw_output", string(output))
	if err != nil {
		log.WithError(err).Error("command execution error")
	}

	response := msg.RunCommandResponse{Output: output, Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	if err != nil {
		response.Success = false
		var exitErr *exec.ExitError
		switch {
		case ctx.Err() == context.DeadlineExceeded:
			response.Error = "Command timed out"
			response.ExitCode = 124
		case errors.As(err, &exitErr) && exitErr.ExitCode() > 0:
			response.Error = err.Error()
			response.ExitCode = exitErr.ExitCode()
		default:
			response.Error = err.Error()
			response.ExitCode = 1
		}
	} else {
		response.Success = true
	}

	log.Debug("run command execution completed", "shell", selectedShell, "command", shellCmd, "output_bytes", len(response.Output), "success", response.Success, "error", response.Error)

	if err := msg.WriteMessage(stream, &response); err != nil {
		log.WithError(err).Error("failed to send run command response")
		return
	}

	log.Debug("response sent successfully")
}

// buildShellCommand makes the string the shell runs. The command goes in as
// given, so a single command string can use pipes and redirection; each
// argument is quoted so it reaches the program as one argument, unchanged.
func buildShellCommand(command string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	if command != "" {
		parts = append(parts, command)
	}
	for _, arg := range args {
		parts = append(parts, shellQuote(arg))
	}
	return strings.Join(parts, " ")
}

// shellQuote quotes s for a POSIX shell, leaving plain words as they are.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	plain := true
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("@%+=:,./_-", r)) {
			plain = false
			break
		}
	}
	if plain {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// lockedWriter serialises writes from the stdout and stderr copiers into
// the interleaved output.
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}
