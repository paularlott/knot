package proxy

import (
	"fmt"
	"net"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

func RunTCPForwarder(proxyServerURL string, listen string, service string, port int) {
  log.Info().Msgf("Listening on %s", listen)
  log.Info().Msgf("Forwarding to %s", service)

  // Build dial address
  dialURL := fmt.Sprintf("%s/forward-port/%s/%d", proxyServerURL, service, port)

	tcpConnection, err := net.Listen("tcp", listen)
	if err != nil {
    log.Fatal().Msgf("Error while opening local port: %s", err.Error())
	}
	defer tcpConnection.Close()

  for {
    tcpConn, err := tcpConnection.Accept()
		if err != nil {
			log.Error().Msgf("Could not accept the connection: %s", err.Error())
			continue
		}

    // Create websocket connection
    wsConn, _, err := websocket.DefaultDialer.Dial(dialURL, nil)
    if err != nil {
      tcpConn.Close()
      log.Fatal().Msgf("Error while dialing: %s", err.Error())
    }
    copier := NewCopier(tcpConn, wsConn)
    go copier.Run()
  }
}
