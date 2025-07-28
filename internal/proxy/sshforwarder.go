package proxy

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	"github.com/paularlott/knot/internal/util"

	"github.com/gorilla/websocket"
)

func RunSSHForwarderViaAgent(proxyServerURL, space, token string, skipTLSVerify bool) error {
	return forwardSSH(fmt.Sprintf("%s/proxy/spaces/%s/ssh/", proxyServerURL, space), token, skipTLSVerify)
}

func forwardSSH(dialURL, token string, skipTLSVerify bool) error {
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
		// If not authorized then tell user
		if response != nil && (response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden) {
			return fmt.Errorf("no permission to use SSH")
		}

		return fmt.Errorf("ssh: error while connecting: %s", err.Error())
	}

	copier := util.NewCopier(nil, wsConn)
	copier.Run()

	return nil
}
