package tunnel_server

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/agentapi/agent_client"
	"github.com/paularlott/knot/internal/wsconn"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/yamux"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

func ConnectAndForward(wsUrl string, protocol string, port uint16, tunnelName string, hostname string) {

	client := apiclient.NewClient(viper.GetString("client.server"), viper.GetString("client.token"), viper.GetBool("tls_skip_verify"))

	// Get the current user
	user, err := client.WhoAmI()
	if err != nil {
		fmt.Println("Error getting user: ", err)
		os.Exit(1)
	}

	log.Info().Msgf("Starting tunnel: %s--%s to port %d", user.Username, tunnelName, port)

	go func() {
		for {

			// Open the websocket
			header := http.Header{"Authorization": []string{fmt.Sprintf("Bearer %s", viper.GetString("client.token"))}}
			dialer := websocket.DefaultDialer
			dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: viper.GetBool("tls_skip_verify")}
			dialer.HandshakeTimeout = 5 * time.Second
			ws, response, err := dialer.Dial(wsUrl+"/tunnel/server/"+tunnelName, header)
			if err != nil {
				if response != nil {
				}
				if response.StatusCode == http.StatusUnauthorized {
					log.Fatal().Msg("Failed to authenticate with server, check permissions")
				} else if response.StatusCode == http.StatusNotFound {
					log.Fatal().Msg("Server does not support tunnels")
				} else if response.StatusCode == http.StatusForbidden {
					log.Fatal().Msg("Tunnels are not available on your account")
				} else if response.StatusCode == http.StatusServiceUnavailable {
					log.Fatal().Msg("Tunnel limit reached")
				}

				log.Error().Msgf("tunnel: error while opening websocket: %s", err)
				time.Sleep(3 * time.Second)
				continue
			}

			// Open the mux session
			localConn := wsconn.New(ws)
			muxSession, err := yamux.Client(localConn, &yamux.Config{
				AcceptBacklog:          256,
				EnableKeepAlive:        true,
				KeepAliveInterval:      30 * time.Second,
				ConnectionWriteTimeout: 10 * time.Second,
				MaxStreamWindowSize:    256 * 1024,
				StreamCloseTimeout:     5 * time.Minute,
				StreamOpenTimeout:      75 * time.Second,
				LogOutput:              io.Discard,
				//Logger:                 logger.NewMuxLogger(),
			})
			if err != nil {
				log.Error().Msgf("tunnel: creating mux session: %v", err)
				ws.Close()
				time.Sleep(3 * time.Second)
				continue
			}

			// Loop forever waiting for connections on the mux session
			for {
				// Accept a new connection
				stream, err := muxSession.Accept()
				if err != nil {
					log.Error().Msgf("tunnel: accepting connection: %v", err)

					// In the case of errors, destroy the session and start over
					muxSession.Close()
					ws.Close()
					time.Sleep(3 * time.Second)

					break
				}

				go handleTunnelStream(stream, protocol, port, hostname)
			}
		}
	}()
}

func handleTunnelStream(stream net.Conn, protocol string, port uint16, hostname string) {
	defer stream.Close()

	if protocol == "http" {
		agent_client.ProxyTcp(stream, fmt.Sprintf("%d", port))
	} else if protocol == "https" {
		agent_client.ProxyTcpTls(stream, fmt.Sprintf("%d", port), hostname)
	}
}
