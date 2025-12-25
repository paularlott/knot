package proxy

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/paularlott/knot/internal/wsconn"

	"github.com/gorilla/websocket"
	"github.com/paularlott/knot/internal/log"
)

func RunTCPForwarderViaAgent(proxyServerURL, listen, space string, port int, token string, skipTLSVerify bool) {
	RunTCPForwarderViaAgentWithContext(context.Background(), proxyServerURL, listen, space, port, token, skipTLSVerify)
}

func RunTCPForwarderViaAgentWithContext(ctx context.Context, proxyServerURL, listen, space string, port int, token string, skipTLSVerify bool) net.Listener {
	logger := log.WithGroup("tcp")

	// Convert http/https to ws/wss
	wsURL := proxyServerURL
	if strings.HasPrefix(proxyServerURL, "https://") {
		wsURL = "wss://" + proxyServerURL[8:]
	} else if strings.HasPrefix(proxyServerURL, "http://") {
		wsURL = "ws://" + proxyServerURL[7:]
	}

	logger.Info("connecting to agent via server at", "proxyServerURL", wsURL)
	return forwardTCPWithContext(ctx, fmt.Sprintf("%s/proxy/spaces/%s/port/%d", wsURL, space, port), token, listen, skipTLSVerify)
}

func forwardTCPWithContext(ctx context.Context, dialURL, token, listen string, skipTLSVerify bool) net.Listener {
	logger := log.WithGroup("tcp")
	tcpConnection, err := net.Listen("tcp", listen)
	if err != nil {
		logger.WithError(err).Error("error while opening local port")
		return nil
	}

	go func() {
		defer tcpConnection.Close()

		// Include auth header if given
		var header http.Header
		if token != "" {
			header = http.Header{"Authorization": []string{fmt.Sprintf("Bearer %s", token)}}
		} else {
			header = nil
		}

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			tcpConn, err := tcpConnection.Accept()
			if err != nil {
				logger.WithError(err).Error("could not accept the connection:")
				return
			}

			// Create websocket connection
			dialer := websocket.DefaultDialer
			dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: skipTLSVerify}
			dialer.HandshakeTimeout = 5 * time.Second
			wsConn, response, err := dialer.Dial(dialURL, header)
			if err != nil {
				if response != nil && response.StatusCode == http.StatusUnauthorized {
					logger.Error("tcp", response.Status)
				} else if response != nil && response.StatusCode == http.StatusForbidden {
					logger.Error("proxy of remote port is not allowed")
				} else {
					logger.WithError(err).Error("error while dialing:")
				}

				tcpConn.Close()
				continue
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
	}()

	return tcpConnection
}
