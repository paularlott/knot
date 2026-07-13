package agent_client

import (
	"context"
	"fmt"
	"io"
	"net"

	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/scriptling/extlibs"
	"github.com/paularlott/scriptling/extlibs/agent"
	"github.com/paularlott/scriptling/object"
)

func handleExecuteScriptStream(stream net.Conn, execMsg msg.ExecuteScriptStreamMessage) {
	log.Trace("executing script stream")

	cfg := config.GetAgentConfig()
	if cfg.DisableSpaceIO {
		stream.Close()
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
				if msg == "stop" {
					cancel()
					stdinW.Close()
					return
				}
				if msg == "stdin_eof" {
					// stdin is closed — close the pipe so input() gets EOF,
					// but do NOT cancel the context. The script continues to
					// run; only "stop" (Ctrl+C) cancels execution.
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

	env, cleanup, err := scriptPool.Acquire()
	if err != nil {
		log.WithError(err).Error("failed to acquire scriptling env from pool")
		stream.Close()
		return
	}
	defer scriptPool.Release(env, cleanup)

	// Per-call I/O setup: point the env at this connection's stream/pipe.
	env.SetOutputWriter(sw)
	env.SetInputReader(stdinR)
	extlibs.RegisterSysLibrary(env, execMsg.Arguments, stdinR)
	env.SetObjectVar("input", extlibs.NewInputBuiltin(stdinR))

	registerConsoleStub(env, stream, controlIn)
	agent.RegisterInteract(env)

	result, err := env.EvalWithContext(ctx, execMsg.Content)

	exitCode := 0
	if ex, ok := object.AsException(result); ok && ex.IsSystemExit() {
		exitCode = ex.GetExitCode()
	} else if err != nil {
		exitCode = 1
		fmt.Fprintln(sw, err)
		log.WithError(err).Error("script execution failed")
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
