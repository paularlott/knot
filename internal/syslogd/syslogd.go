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

		// If the message has a priority then extract it and get the severity from it, priority mod 8
		priority := 0
		severity := 0
		_, err = fmt.Sscanf(message, "<%d>", &priority)
		if err == nil {
			severity = priority % 8
		}

		/**
		 * Map the severity to a log level
		 * 0: Emergency (system is unusable)
		 * 1: Alert (action must be taken immediately)
		 * 2: Critical (critical conditions)
		 * 3: Error (error conditions)
		 * 4: Warning (warning conditions)
		 * 5: Notice (normal but significant condition)
		 * 6: Informational (informational messages)
		 * 7: Debug (debug-level messages)
		 */

		// Severity mapping matches the numeric-level mapping used by the
		// VictoriaLogs endpoint: 0-3 (emergency … error) map to error,
		// 4-6 (warning … informational) round down to info — knot has no
		// warn level — and 7 (debug) maps to debug.
		var logLevel msg.LogLevel
		if severity >= 7 {
			logLevel = msg.LogLevelDebug
		} else if severity >= 4 {
			logLevel = msg.LogLevelInfo
		} else {
			logLevel = msg.LogLevelError
		}

		// Forward the message to the server. Records arriving over syslog
		// carry no service of their own, so they get the knot fallback
		// service (source:knot still sifts them from other sources).
		agentClient.SendLogMessage("knot_syslog", logLevel, message)
	}
}
