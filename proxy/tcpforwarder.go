package proxy

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/paularlott/knot/util"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

func RunTCPForwarderViaProxy(proxyServerURL string, token string, listen string, service string, port int) {
	log.Info().Msgf("tcp: listening on %s forwarding to %s via %s", listen, service, proxyServerURL)
	forwardTCP(fmt.Sprintf("%s/proxy/port/%s/%d", proxyServerURL, service, port), token, listen)
}

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

	for {
		tcpConn, err := tcpConnection.Accept()
		if err != nil {
			log.Error().Msgf("tcp: could not accept the connection: %s", err.Error())
			continue
		}

		// Create websocket connection
		dialer := websocket.DefaultDialer
		dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: viper.GetBool("tls_skip_verify")}
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
		copier := util.NewCopier(tcpConn, wsConn)
		go copier.Run()
	}
}
