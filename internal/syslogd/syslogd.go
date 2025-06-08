package syslogd

import (
	"fmt"
	"net"

	"github.com/paularlott/knot/internal/agentapi/agent_client"
	"github.com/paularlott/knot/internal/agentapi/msg"

	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

// Very simple syslogd server to collect logs and pass them to the server
func StartSyslogd(agentClient *agent_client.AgentClient) {
	addr := net.UDPAddr{
		Port: viper.GetInt("agent.syslog_port"),
		IP:   net.ParseIP("127.0.0.1"),
	}
	conn, err := net.ListenUDP("udp", &addr)
	if err != nil {
		log.Fatal().Msgf("syslogd: failed to set up UDP server: %v", err)
	}
	defer conn.Close()

	log.Info().Msgf("syslogd: server listening on port 514")
	buffer := make([]byte, 8192)
	for {
		n, _, err := conn.ReadFromUDP(buffer)
		if err != nil {
			log.Info().Msgf("syslogd: error reading from UDP: %v", err)
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

		var logLevel msg.LogLevel
		if severity >= 7 {
			logLevel = msg.LogLevelDebug
		} else if severity >= 5 || severity <= 0 {
			logLevel = msg.LogLevelInfo
		} else {
			logLevel = msg.LogLevelError
		}

		// Forward the message to the server
		agentClient.SendLogMessage("syslog", logLevel, message)
	}
}
