package proxy

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/paularlott/knot/internal/wsconn"

	"github.com/gorilla/websocket"
	"github.com/paularlott/knot/internal/log"
)

func RunTCPForwarderViaAgent(proxyServerURL, listen, space string, port int, token string, skipTLSVerify bool) {
	logger := log.WithGroup("tcp")
	logger.Info("connecting to agent via server at", "proxyServerURL", proxyServerURL)
	forwardTCP(fmt.Sprintf("%s/proxy/spaces/%s/port/%d", proxyServerURL, space, port), token, listen, skipTLSVerify)
}

func forwardTCP(dialURL, token, listen string, skipTLSVerify bool) {
	logger := log.WithGroup("tcp")
	tcpConnection, err := net.Listen("tcp", listen)
	if err != nil {
		logger.WithError(err).Fatal("error while opening local port")
	}
	defer tcpConnection.Close()

	// Include auth header if given
	var header http.Header
	if token != "" {
		header = http.Header{"Authorization": []string{fmt.Sprintf("Bearer %s", token)}}
	} else {
		header = nil
	}

	for {
		tcpConn, err := tcpConnection.Accept()
		if err != nil {
			logger.WithError(err).Error("could not accept the connection:")
			continue
		}

		// Create websocket connection
		dialer := websocket.DefaultDialer
		dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: skipTLSVerify}
		dialer.HandshakeTimeout = 5 * time.Second
		wsConn, response, err := dialer.Dial(dialURL, header)
		if err != nil {
			if response != nil && response.StatusCode == http.StatusUnauthorized {
				logger.Fatal("tcp", response.Status)
			} else if response != nil && response.StatusCode == http.StatusForbidden {
				logger.Fatal("proxy of remote port is not allowed")
			} else {
				logger.WithError(err).Fatal("error while dialing:")
			}

			tcpConn.Close()
			os.Exit(1)
		}

		conn := wsconn.New(wsConn)

		go func() {
			// copy data between code server and server
			var once sync.Once
			closeBoth := func() {
				conn.Close()
				tcpConn.Close()
			}

			// Copy from client to tunnel
			go func() {
				_, _ = io.Copy(conn, tcpConn)
				once.Do(closeBoth)
			}()

			// Copy from tunnel to client
			_, _ = io.Copy(tcpConn, conn)
			once.Do(closeBoth)
		}()
	}
}
