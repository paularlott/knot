package proxy

import (
	"fmt"
	"net/http"
	"os"

	"github.com/gorilla/websocket"
	"github.com/paularlott/knot/util"
	"github.com/rs/zerolog/log"
)

func RunSSHForwarderViaProxy(proxyServerURL string, token string, service string, port int) {
  log.Debug().Msgf("ssh: connecting to proxy server at: %s", proxyServerURL)
  forwardSSH(fmt.Sprintf("%s/proxy/port/%s/%d", proxyServerURL, service, port), token)
}

func RunSSHForwarderViaAgent(proxyServerURL string, space string, token string) {
  log.Debug().Msgf("ssh: connecting to agent via server at: %s", proxyServerURL)
  forwardSSH(fmt.Sprintf("%s/proxy/spaces/%s/ssh/", proxyServerURL, space), token)
}

func forwardSSH(dialURL string, token string) {
  // Include auth header if given
  var header http.Header
  if token != "" {
    header = http.Header{"Authorization": []string{fmt.Sprintf("Bearer %s", token)}}
  } else {
    header = nil
  }

  // Create websocket connection
  wsConn, _, err := websocket.DefaultDialer.Dial(dialURL, header)
  if err != nil {
    log.Fatal().Msgf("ssh: error while dialing: %s", err.Error())
    os.Exit(1)
  }

  copier := util.NewCopier(nil, wsConn)
  copier.Run()
}
