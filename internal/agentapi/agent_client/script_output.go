package agent_client

import (
	"bytes"
	"io"
	"os"
	"sync"

	"github.com/paularlott/knot/internal/agentapi/msg"
)

// scriptOutputLineMax is the longest line of a script's output sent to the
// space's log as one record; a longer one is sent in pieces.
const scriptOutputLineMax = 16 * 1024

// scriptOutput takes what a startup or shutdown script prints: as it is to
// the agent's output (stderr, the container's log), and a line at a time to
// the space's log.
type scriptOutput struct {
	client  *AgentClient
	service string
	stderr  io.Writer

	mu  sync.Mutex
	buf []byte
}

func newScriptOutput(client *AgentClient, service string) *scriptOutput {
	return &scriptOutput{client: client, service: service, stderr: os.Stderr}
}

func (w *scriptOutput) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.stderr.Write(p)
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		w.send(w.buf[:i])
		w.buf = w.buf[i+1:]
	}
	for len(w.buf) >= scriptOutputLineMax {
		w.send(w.buf[:scriptOutputLineMax])
		w.buf = w.buf[scriptOutputLineMax:]
	}
	return len(p), nil
}

// Close sends what is left of a last line without a newline.
func (w *scriptOutput) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buf) > 0 {
		w.send(w.buf)
		w.buf = nil
	}
	return nil
}

func (w *scriptOutput) send(line []byte) {
	if w.client == nil {
		return
	}
	_ = w.client.SendLogMessage(w.service, msg.LogLevelInfo, string(bytes.TrimRight(line, "\r")))
}
