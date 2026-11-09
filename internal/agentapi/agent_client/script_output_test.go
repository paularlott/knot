package agent_client

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/log"
)

func drainLog(c *AgentClient) []*msg.LogMessage {
	var out []*msg.LogMessage
	for {
		select {
		case m := <-c.logChannel:
			out = append(out, m)
		default:
			return out
		}
	}
}

// A startup script's output reaches the agent's output as it is, and the
// space's log a line at a time.
func TestScriptOutput(t *testing.T) {
	c := &AgentClient{logChannel: make(chan *msg.LogMessage, 100)}
	var stderr bytes.Buffer
	w := newScriptOutput(c, "knot_script")
	w.stderr = &stderr

	w.Write([]byte("first line\nsecond "))
	w.Write([]byte("half\r\nthird"))
	w.Write([]byte(strings.Repeat("x", scriptOutputLineMax+10)))
	w.Close()

	if want := "first line\nsecond half\r\nthird" + strings.Repeat("x", scriptOutputLineMax+10); stderr.String() != want {
		t.Errorf("the agent's output was not the script's as it is: %q", stderr.String()[:40])
	}
	var lines []string
	for _, m := range drainLog(c) {
		if m.Service != "knot_script" || m.Level != msg.LogLevelInfo {
			t.Errorf("record %+v", m)
		}
		lines = append(lines, m.Message)
	}
	// "third" and the long run: one record of the most a line may be, then
	// what is left.
	if len(lines) != 4 || lines[0] != "first line" || lines[1] != "second half" || len(lines[2]) != scriptOutputLineMax || lines[3] != strings.Repeat("x", len("third")+10) {
		t.Errorf("space log lines: %d, first %q, second %q, %d, %q", len(lines), lines[0], lines[1], len(lines[2]), lines[len(lines)-1])
	}
	if !strings.HasPrefix(lines[2], "third") {
		t.Errorf("the long line did not start where the last ended: %q", lines[2][:10])
	}
}

// The space's logger writes to the agent's output too, once, and the copy
// log.Tee makes writes to the space's log only, so nothing is written
// twice.
func TestAgentClientLoggerWritesBoth(t *testing.T) {
	var stderr bytes.Buffer
	log.Configure("info", "console", &stderr)
	t.Cleanup(func() { log.Configure("info", "console", nil); log.Tee(nil) })
	c := &AgentClient{logChannel: make(chan *msg.LogMessage, 100)}
	log.Tee(NewSpaceLogCopy(c, "knot_agent"))

	NewAgentClientLogger(c, "knot_sync").With("job", "src").Info("uploaded", "file", "a.txt")
	log.Info("agent record", "k", "v")

	out := regexp.MustCompile("\x1b\\[[0-9;]*m").ReplaceAllString(stderr.String(), "")
	if strings.Count(out, "uploaded") != 1 || !strings.Contains(out, "service=knot_sync") || !strings.Contains(out, "job=src") {
		t.Errorf("the agent's output:\n%s", out)
	}
	if strings.Count(out, "agent record") != 1 {
		t.Errorf("an agent record written %d times:\n%s", strings.Count(out, "agent record"), out)
	}
	got := map[string]string{}
	for _, m := range drainLog(c) {
		got[m.Service] = m.Message
	}
	if !strings.Contains(got["knot_sync"], "uploaded") || !strings.Contains(got["knot_sync"], "file=a.txt") || !strings.Contains(got["knot_agent"], "agent record k=v") || len(got) != 2 {
		t.Errorf("the space's log got %v", got)
	}
}
