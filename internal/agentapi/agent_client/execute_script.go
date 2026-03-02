package agent_client

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/logger"
	"github.com/paularlott/scriptling"
)

var agentClient *AgentClient

func SetAgentClient(client *AgentClient) {
	agentClient = client
}

func handleExecuteScript(stream net.Conn, execMsg msg.ExecuteScriptMessage) {
	log.Debug("executing script", "is_system_call", execMsg.IsSystemCall)

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

	// Scripts run without timeout - they run until completion or are cancelled
	ctx := context.Background()

	var client *apiclient.ApiClient
	var userId string

	if agentClient != nil {
		server := agentClient.GetServerURL()
		token := agentClient.GetAgentToken()
		if server != "" && token != "" {
			var err error
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

	var env *scriptling.Scriptling
	var err error

	// Use custom logger when agent client is available (for space startup and run-script commands)
	var customLogger logger.Logger = nil
	if agentClient != nil {
		customLogger = NewAgentClientLogger(agentClient, "script")
	}
	env, err = service.NewRemoteScriptlingEnv(execMsg.Arguments, client, userId, customLogger, execMsg.IsSystemCall)

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

	result, err := env.EvalWithContext(ctx, execMsg.Content)
	exitCode, output, evalErr := service.HandleScriptResult(result, err, env.GetOutput())

	response := msg.ExecuteScriptResponse{
		Success:  evalErr == nil && exitCode == 0,
		ExitCode: exitCode,
		Output:   output,
	}
	if evalErr != nil {
		response.Error = evalErr.Error()
		log.WithError(evalErr).Error("script execution failed")
	}

	if err := msg.WriteMessage(stream, &response); err != nil {
		log.WithError(err).Error("failed to send script execution response")
		return
	}

	log.Debug("script execution completed", "success", response.Success)
}
