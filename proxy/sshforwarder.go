package proxy

import (
	"fmt"
	"os"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

func RunSSHForwarder(proxyServerURL string, service string, port int) {
  log.Info().Msgf("Connecting to proxy server at: %s", proxyServerURL)

  // Build dial address
  dialURL := fmt.Sprintf("%s/forward-port/%s/%d", proxyServerURL, service, port)

  for {
    // Create websocket connection
    wsConn, _, err := websocket.DefaultDialer.Dial(dialURL, nil)
    if err != nil {
      log.Fatal().Msgf("Error while dialing: %s", err.Error())
      os.Exit(1)
    }

    copier := NewCopier(nil, wsConn)
    copier.Run()
  }
}
