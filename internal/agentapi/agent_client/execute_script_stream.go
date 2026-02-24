package agent_client

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/scriptling/extlibs/agent"
)

func handleExecuteScriptStream(stream net.Conn, execMsg msg.ExecuteScriptStreamMessage) {
	log.Debug("executing script stream")

	cfg := config.GetAgentConfig()
	if cfg.DisableSpaceIO {
		stream.Close()
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
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

	// controlIn receives inbound FrameControl messages for the console stub
	controlIn := make(chan string, 16)

	// Pipe for inbound stdio frames → script stdin
	stdinR, stdinW := io.Pipe()

	// Read inbound frames from the mux stream
	go func() {
		defer stdinW.Close()
		for {
			frameType, payload, err := ReadFrame(stream)
			if err != nil {
				close(controlIn)
				return
			}
			switch frameType {
			case FrameStdio:
				if _, err := stdinW.Write(payload); err != nil {
					return
				}
			case FrameControl:
				msg := string(payload)
				if msg == "stop" || msg == "stdin_eof" {
					cancel()
					stdinW.Close()
					return
				}
				select {
				case controlIn <- msg:
				default:
				}
			}
		}
	}()

	// scriptWriter wraps the mux stream, framing all writes as FrameStdio
	sw := &stdioWriter{w: stream}

	customLogger := NewAgentClientLogger(agentClient, "script")
	env, err := service.NewRemoteStreamingScriptlingEnv(execMsg.Arguments, client, userId, customLogger, sw, stdinR)
	if err != nil {
		log.WithError(err).Error("failed to create scriptling environment")
		stream.Close()
		return
	}

	registerConsoleStub(env, stream, controlIn)
	agent.RegisterInteract(env)

	result, err := env.EvalWithContext(ctx, execMsg.Content)
	exitCode, output, evalErr := service.HandleScriptResult(result, err, "")

	if evalErr != nil {
		log.WithError(evalErr).Error("script execution failed")
	} else if output != "" {
		fmt.Fprintln(sw, output)
	}

	if err := WriteFrame(stream, FrameControl, []byte(fmt.Sprintf("exit:%d", exitCode))); err != nil {
		log.WithError(err).Error("failed to write exit frame")
	}
	stream.Close()
	log.Debug("script stream execution completed", "exit_code", exitCode)
}

// stdioWriter wraps an io.Writer, framing all writes as FrameStdio frames.
type stdioWriter struct {
	w io.Writer
}

func (s *stdioWriter) Write(p []byte) (int, error) {
	if err := WriteFrame(s.w, FrameStdio, p); err != nil {
		return 0, err
	}
	return len(p), nil
}
