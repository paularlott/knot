package syslogd

import (
	"testing"

	"github.com/paularlott/knot/internal/agentapi/msg"
)

func TestLevelFor(t *testing.T) {
	cases := []struct {
		message string
		want    msg.LogLevel
	}{
		// no priority: plain lines (e.g. web access logs) are informational
		{`127.0.0.1 - - [10/Oct/2026:17:35:46 +0000] "GET / HTTP/1.1" 200 27718`, msg.LogLevelInfo},
		{"", msg.LogLevelInfo},
		{"<x>not a number", msg.LogLevelInfo},
		// a real priority keeps its severity
		{"<0>kernel: panic", msg.LogLevelError},           // kern.emerg
		{"<27>php-fpm: NOTICE: ready", msg.LogLevelError}, // daemon.err
		{"<28>daemon warning", msg.LogLevelInfo},          // daemon.warning
		{"<134>root: Starting Caddy..", msg.LogLevelInfo}, // local0.info
		{"<78>cron[97]: (CRON) INFO", msg.LogLevelInfo},   // cron.info
		{"<15>user debug", msg.LogLevelDebug},             // user.debug
	}
	for _, c := range cases {
		if got := levelFor(c.message); got != c.want {
			t.Errorf("levelFor(%q) = %v, want %v", c.message, got, c.want)
		}
	}
}
