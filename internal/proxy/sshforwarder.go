package proxy

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/util"

	"github.com/gorilla/websocket"
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
	cfg := config.GetServerConfig()
	dialer := websocket.DefaultDialer
	dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: cfg.TLS.SkipVerify}
	dialer.HandshakeTimeout = 5 * time.Second
	wsConn, response, err := dialer.Dial(dialURL, header)
	if err != nil {

		// If not autorized then tell user
		if response != nil && response.StatusCode == http.StatusUnauthorized {
			log.Fatal().Msgf("ssh: %s", response.Status)
		} else {
			log.Fatal().Msgf("ssh: error while dialing: %s", err.Error())
		}
		os.Exit(1)
	}

	copier := util.NewCopier(nil, wsConn)
	copier.Run()
}
