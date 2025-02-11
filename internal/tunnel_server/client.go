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

	// Get the tunnel domain
	tunnelDomain, _, err := client.GetTunnelDomain()
	if err != nil {
		fmt.Println("Error getting tunnel domain: ", err)
		os.Exit(1)
	}

	log.Info().Msgf("https://%s--%s%s -> %s://localhost:%d", user.Username, tunnelName, tunnelDomain, protocol, port)

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

				log.Error().Msgf("Error while opening websocket: %s", err)
				time.Sleep(3 * time.Second)
				continue
			}

			// Open the mux session
			localConn := wsconn.New(ws)
			muxSession, err := yamux.Client(localConn, &yamux.Config{
				AcceptBacklog:          256,
				EnableKeepAlive:        true,
				KeepAliveInterval:      30 * time.Second,
				ConnectionWriteTimeout: 2 * time.Second,
				MaxStreamWindowSize:    256 * 1024,
				StreamCloseTimeout:     3 * time.Minute,
				StreamOpenTimeout:      3 * time.Second,
				LogOutput:              io.Discard,
				//Logger:                 logger.NewMuxLogger(),
			})
			if err != nil {
				log.Error().Msgf("Creating mux session: %v", err)
				ws.Close()
				time.Sleep(3 * time.Second)
				continue
			}

			// Loop forever waiting for connections on the mux session
			for {
				// Accept a new connection
				stream, err := muxSession.Accept()
				if err != nil {
					log.Error().Msgf("Accepting connection: %v", err)

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

	// Read the 1st byte to determine if this is a new connection or terminate
	buf := make([]byte, 1)
	_, err := stream.Read(buf)
	if err != nil {
		log.Error().Msgf("Error reading from stream: %v", err)
		return
	}

	// If the byte is 0, then close the stream
	if buf[0] == 0 {
		log.Fatal().Msg("Received close signal from server")
		return
	}

	if protocol == "http" {
		agent_client.ProxyTcp(stream, fmt.Sprintf("%d", port))
	} else if protocol == "https" {
		agent_client.ProxyTcpTls(stream, fmt.Sprintf("%d", port), hostname)
	}
}
