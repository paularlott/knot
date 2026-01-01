package agent_client

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/service"
)

var agentClient *AgentClient

func SetAgentClient(client *AgentClient) {
	agentClient = client
}

func handleExecuteScript(stream net.Conn, execMsg msg.ExecuteScriptMessage) {
	log.Debug("executing script", "timeout", execMsg.Timeout, "is_system_call", execMsg.IsSystemCall)

	// Check if user scripts are disabled (system scripts always allowed)
	if !execMsg.IsSystemCall {
		cfg := config.GetAgentConfig()
		if cfg.DisableSpaceIO {
			response := msg.ExecuteScriptResponse{
				Success: false,
				Error:   "Script execution disabled by agent configuration",
			}
			if err := msg.WriteMessage(stream, &response); err != nil {
				log.WithError(err).Error("failed to send disabled response")
			}
			return
		}
	}

	timeout := time.Duration(execMsg.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 120 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var client *apiclient.ApiClient
	var userId string

	if agentClient != nil {
		server, token, err := agentClient.SendRequestToken()
		if err == nil {
			client, err = apiclient.NewClient(server, token, true)
			if err == nil {
				client.SetTimeout(6 * time.Minute)
				user, err := client.WhoAmI(ctx)
				if err == nil {
					userId = user.Id
				}
			}
		}
	}

	env, err := service.NewRemoteScriptlingEnv(execMsg.Arguments, execMsg.Libraries, client, userId)
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
