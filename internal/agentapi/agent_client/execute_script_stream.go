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
	"github.com/paularlott/scriptling/extlibs"
)

func handleExecuteScriptStream(stream net.Conn, execMsg msg.ExecuteScriptStreamMessage) {
	log.Debug("executing script stream", "timeout", execMsg.Timeout)

	cfg := config.GetAgentConfig()
	if cfg.DisableSpaceIO {
		stream.Close()
		return
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

	result, evalErr := env.EvalWithContext(ctx, execMsg.Content)

	exitCode := 0
	if evalErr != nil {
		if sysExit, ok := extlibs.GetSysExitCode(evalErr); ok {
			exitCode = sysExit.Code
		} else {
			log.WithError(evalErr).Error("script execution failed")
			exitCode = 1
		}
	} else if result != nil && result.Inspect() != "None" {
		fmt.Fprintln(stream, result.Inspect())
	}

	// Send exit code as final message before closing
	fmt.Fprintf(stream, "\nexit:%d\n", exitCode)
	stream.Close()
	log.Debug("script stream execution completed", "exit_code", exitCode)
}
