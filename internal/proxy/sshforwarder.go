package proxy

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/paularlott/knot/internal/util"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

func RunSSHForwarderViaAgent(proxyServerURL, space, token string, skipTLSVerify bool) {
	log.Debug().Msgf("ssh: connecting to agent via server at: %s", proxyServerURL)
	forwardSSH(fmt.Sprintf("%s/proxy/spaces/%s/ssh/", proxyServerURL, space), token, skipTLSVerify)
}

func forwardSSH(dialURL, token string, skipTLSVerify bool) {
	// Include auth header if given
	var header http.Header
	if token != "" {
		header = http.Header{"Authorization": []string{fmt.Sprintf("Bearer %s", token)}}
	} else {
		header = nil
	}

	// Create websocket connection
	dialer := websocket.DefaultDialer
	dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: skipTLSVerify}
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
