package syslogd

import (
	"net"

	"github.com/paularlott/knot/internal/agentapi/agent_client"
	"github.com/paularlott/knot/internal/agentapi/msg"

	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

// Very simple syslogd server to collect logs and pass them to the server
func StartSyslogd() {
	addr := net.UDPAddr{
		Port: viper.GetInt("agent.syslog_port"),
		IP:   net.ParseIP("127.0.0.1"),
	}
	conn, err := net.ListenUDP("udp", &addr)
	if err != nil {
		log.Info().Msgf("syslogd: failed to set up UDP server: %v", err)
	}
	defer conn.Close()

	log.Info().Msgf("syslogd: server listening on port 514")
	buffer := make([]byte, 1024)
	for {
		n, _, err := conn.ReadFromUDP(buffer)
		if err != nil {
			log.Info().Msgf("syslogd: error reading from UDP: %v", err)
			continue
		}

		// Forward the message to the server
		agent_client.SendLogMessage(msg.MSG_LOG_SYSLOG, string(buffer[:n]))
	}
}
