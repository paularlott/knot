package syslogd

import (
	"fmt"
	"net"

	"github.com/paularlott/knot/internal/agentapi/agent_client"
	"github.com/paularlott/knot/internal/agentapi/msg"

	"github.com/paularlott/knot/internal/log"
)

// Very simple syslogd server to collect logs and pass them to the server
func StartSyslogd(agentClient *agent_client.AgentClient, syslogPort int) {
	logger := log.WithGroup("syslogd")

	addr := net.UDPAddr{
		Port: syslogPort,
		IP:   net.ParseIP("127.0.0.1"),
	}
	conn, err := net.ListenUDP("udp", &addr)
	if err != nil {
		logger.Fatal("failed to set up UDP server:", "err", err)
	}
	defer conn.Close()

	logger.Info("server listening on port", "port", syslogPort)
	buffer := make([]byte, 8192)
	for {
		n, _, err := conn.ReadFromUDP(buffer)
		if err != nil {
			logger.WithError(err).Info("error reading from UDP:")
			continue
		}

		message := string(buffer[:n])

		logLevel := levelFor(message)

		// Forward the message to the server. Records arriving over syslog
		// carry no service of their own, so they get the knot fallback
		// service (source:knot still sifts them from other sources).
		agentClient.SendLogMessage("knot_syslog", logLevel, message)
	}
}

// levelFor maps a syslog message's severity (its <priority> mod 8) to a knot
// log level, the same mapping the VictoriaLogs endpoint uses for numeric
// levels:
//
//	0-3 (emergency, alert, critical, error)    -> error
//	4-6 (warning, notice, informational)       -> info (knot has no warn level)
//	7   (debug)                                -> debug
//
// A message without a priority, such as a program writing plain lines to the
// socket (web server access logs), is informational: the syslog default
// (RFC 3164 4.3.3, user.notice), not an emergency.
func levelFor(message string) msg.LogLevel {
	var priority int
	if _, err := fmt.Sscanf(message, "<%d>", &priority); err != nil || priority < 0 {
		return msg.LogLevelInfo
	}
	switch severity := priority % 8; {
	case severity >= 7:
		return msg.LogLevelDebug
	case severity >= 4:
		return msg.LogLevelInfo
	default:
		return msg.LogLevelError
	}
}
