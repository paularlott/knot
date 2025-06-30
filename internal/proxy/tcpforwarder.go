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

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/wsconn"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

func RunTCPForwarderViaAgent(proxyServerURL string, listen string, space string, port int, token string) {
	log.Info().Msgf("tcp: connecting to agent via server at: %s", proxyServerURL)
	forwardTCP(fmt.Sprintf("%s/proxy/spaces/%s/port/%d", proxyServerURL, space, port), token, listen)
}

func forwardTCP(dialURL string, token string, listen string) {
	tcpConnection, err := net.Listen("tcp", listen)
	if err != nil {
		log.Fatal().Msgf("tcp: error while opening local port: %s", err.Error())
	}
	defer tcpConnection.Close()

	// Include auth header if given
	var header http.Header
	if token != "" {
		header = http.Header{"Authorization": []string{fmt.Sprintf("Bearer %s", token)}}
	} else {
		header = nil
	}

	cfg := config.GetServerConfig()
	for {
		tcpConn, err := tcpConnection.Accept()
		if err != nil {
			log.Error().Msgf("tcp: could not accept the connection: %s", err.Error())
			continue
		}

		// Create websocket connection
		dialer := websocket.DefaultDialer
		dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: cfg.TLS.SkipVerify}
		dialer.HandshakeTimeout = 5 * time.Second
		wsConn, response, err := dialer.Dial(dialURL, header)
		if err != nil {
			if response != nil && response.StatusCode == http.StatusUnauthorized {
				log.Fatal().Msgf("tcp: %s", response.Status)
			} else if response != nil && response.StatusCode == http.StatusForbidden {
				log.Fatal().Msgf("tcp: proxy of remote port is not allowed")
			} else {
				log.Fatal().Msgf("tcp: error while dialing: %s", err.Error())
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
