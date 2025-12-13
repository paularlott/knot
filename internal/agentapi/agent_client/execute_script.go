package agent_client

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/service"
)

func handleExecuteScript(stream net.Conn, execMsg msg.ExecuteScriptMessage) {
	log.Debug("executing script", "timeout", execMsg.Timeout)

	timeout := time.Duration(execMsg.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	env, err := service.NewScriptlingEnvWithDiskLibraries(execMsg.Arguments, execMsg.Libraries)
	if err != nil {
		response := msg.ExecuteScriptResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to create scriptling environment: %v", err),
		}
		if err := msg.WriteMessage(stream, &response); err != nil {
			log.WithError(err).Error("failed to send environment creation error response")
		}
		return
	}

	result, evalErr := env.EvalWithContext(ctx, execMsg.Content)

	response := msg.ExecuteScriptResponse{}
	if evalErr != nil {
		response.Success = false
		response.Error = evalErr.Error()
		log.WithError(evalErr).Error("script execution failed")
	} else {
		response.Success = true
		output := env.GetOutput()
		if result != nil && result.Inspect() != "None" {
			if output != "" {
				output += "\n"
			}
			output += result.Inspect()
		}
		response.Output = strings.TrimRight(output, "\n")
	}

	if err := msg.WriteMessage(stream, &response); err != nil {
		log.WithError(err).Error("failed to send script execution response")
		return
	}

	log.Debug("script execution completed", "success", response.Success)
}
