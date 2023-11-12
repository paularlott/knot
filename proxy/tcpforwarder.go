package proxy

import (
	"fmt"
	"net"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

func RunTCPForwarderViaProxy(proxyServerURL string, listen string, service string, port int) {
  log.Info().Msgf("tcp: listening on %s forwarding to %s via %s", listen, service, proxyServerURL)
  forwardTCP(fmt.Sprintf("%s/proxy/port/%s/%d", proxyServerURL, service, port), listen)
}

func RunTCPForwarderViaAgent(proxyServerURL string, listen string, box string, port int) {
  log.Info().Msgf("tcp: connecting to agent via server at: %s", proxyServerURL)
  forwardTCP(fmt.Sprintf("%s/%s/port/%d", proxyServerURL, box, port), listen)
}

func forwardTCP(dialURL string, listen string) {
	tcpConnection, err := net.Listen("tcp", listen)
	if err != nil {
    log.Fatal().Msgf("tcp: error while opening local port: %s", err.Error())
	}
	defer tcpConnection.Close()

  for {
    tcpConn, err := tcpConnection.Accept()
		if err != nil {
			log.Error().Msgf("tcp: could not accept the connection: %s", err.Error())
			continue
		}

    // Create websocket connection
    wsConn, _, err := websocket.DefaultDialer.Dial(dialURL, nil)
    if err != nil {
      tcpConn.Close()
      log.Fatal().Msgf("tcp: error while dialing: %s", err.Error())
    }
    copier := NewCopier(tcpConn, wsConn)
    go copier.Run()
  }
}
