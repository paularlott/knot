package proxy

import (
	"fmt"
	"net/http"
	"os"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

func RunSSHForwarderViaProxy(proxyServerURL string, token string, service string, port int) {
  log.Info().Msgf("ssh: connecting to proxy server at: %s", proxyServerURL)
  forwardSSH(fmt.Sprintf("%s/proxy/port/%s/%d", proxyServerURL, service, port), token)
}

func RunSSHForwarderViaAgent(proxyServerURL string, box string) {
  log.Info().Msgf("ssh: connecting to agent via server at: %s", proxyServerURL)
  forwardSSH(fmt.Sprintf("%s/%s/ssh/", proxyServerURL, box), "")
}

func forwardSSH(dialURL string, token string) {
  // Include auth header if given
  var header http.Header
  if token != "" {
    header = http.Header{"Authorization": []string{fmt.Sprintf("Bearer %s", token)}}
  } else {
    header = nil
  }

  for {
    // Create websocket connection
    wsConn, _, err := websocket.DefaultDialer.Dial(dialURL, header)
    if err != nil {
      log.Fatal().Msgf("ssh: error while dialing: %s", err.Error())
      os.Exit(1)
    }

    copier := NewCopier(nil, wsConn)
    copier.Run()
  }
}
