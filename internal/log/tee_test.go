package log

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/paularlott/logger"
	logslog "github.com/paularlott/logger/slog"
)

// Tee copies what the logger takes at or above its level, through With,
// WithError and WithGroup too; Local writes only to the logger itself.
func TestTee(t *testing.T) {
	var main, copied bytes.Buffer
	Configure("info", "console", &main)
	t.Cleanup(func() { Configure("info", "console", nil); Tee(nil) })
	// Taken before Tee, as package variables and the agent's own are.
	early := WithGroup("early").With("k", "v")
	Tee(logslog.New(logslog.Config{Level: "trace", Format: "console", Writer: &copied}))

	early.Info("from a logger taken before")
	Debug("below the level")
	Info("plain")
	With("job", "src").Warn("with a field")
	WithGroup("sync").Error("in a group")
	GetLogger().Info("through GetLogger")
	Local().Info("local only")

	for _, s := range []string{"plain", "with a field", "job=src", "in a group", "through GetLogger", "local only"} {
		if !strings.Contains(plain(&main), s) {
			t.Errorf("the logger's own output lacks %q:\n%s", s, plain(&main))
		}
	}
	for _, s := range []string{"plain", "with a field", "job=src", "in a group", "through GetLogger", "from a logger taken before", "k=v", "[early]"} {
		if !strings.Contains(plain(&copied), s) {
			t.Errorf("the copy lacks %q:\n%s", s, plain(&copied))
		}
	}
	for _, s := range []string{"below the level", "local only"} {
		if strings.Contains(copied.String(), s) {
			t.Errorf("the copy has %q:\n%s", s, copied.String())
		}
	}

	// A second Tee replaces the copy, not stacks another on it.
	var second bytes.Buffer
	Tee(logslog.New(logslog.Config{Level: "trace", Format: "console", Writer: &second}))
	copied.Reset()
	Info("after the second")
	if strings.Contains(copied.String(), "after the second") || !strings.Contains(second.String(), "after the second") {
		t.Errorf("a second Tee: first copy %q, second %q", copied.String(), second.String())
	}
	var _ logger.Logger = teeLogger{}
}

// At debug, debug records are copied too.
func TestTeeLevel(t *testing.T) {
	var main, copied bytes.Buffer
	Configure("debug", "console", &main)
	t.Cleanup(func() { Configure("info", "console", nil); Tee(nil) })
	Tee(logslog.New(logslog.Config{Level: "trace", Format: "console", Writer: &copied}))
	Debug("a debug record")
	Trace("a trace record")
	if !strings.Contains(copied.String(), "a debug record") || strings.Contains(copied.String(), "a trace record") {
		t.Errorf("at debug the copy has %q", copied.String())
	}
}

var ansi = regexp.MustCompile("\x1b\\[[0-9;]*m")

// plain is what was written without its colours.
func plain(b *bytes.Buffer) string { return ansi.ReplaceAllString(b.String(), "") }
