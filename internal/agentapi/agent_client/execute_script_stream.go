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
)

func handleExecuteScriptStream(stream net.Conn, execMsg msg.ExecuteScriptStreamMessage) {
	log.Debug("executing script stream")

	cfg := config.GetAgentConfig()
	if cfg.DisableSpaceIO {
		stream.Close()
		return
	}

	// Streaming scripts run without timeout - they run until completion or are cancelled
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

	customLogger := NewAgentClientLogger(agentClient, "script")
	env, err := service.NewRemoteStreamingScriptlingEnv(execMsg.Arguments, client, userId, customLogger, stream, stream)
	if err != nil {
		log.WithError(err).Error("failed to create scriptling environment")
		stream.Close()
		return
	}

	result, err := env.EvalWithContext(ctx, execMsg.Content)
	exitCode, output, evalErr := service.HandleScriptResult(result, err, "")

	if evalErr != nil {
		log.WithError(evalErr).Error("script execution failed")
	} else if output != "" {
		fmt.Fprintln(stream, output)
	}

	fmt.Fprintf(stream, "\nexit:%d\n", exitCode)
	stream.Close()
	log.Debug("script stream execution completed", "exit_code", exitCode)
}
