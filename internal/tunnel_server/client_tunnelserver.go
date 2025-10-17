package tunnel_server

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/paularlott/knot/internal/agentapi/agent_client"
	"github.com/paularlott/knot/internal/wsconn"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/yamux"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/logger"
)

const (
	maxConnectionAttempts = 5               // Maximum number of connection attempts before giving up
	connectRetryDelay     = 1 * time.Second // Delay before retrying connection
)

type tunnelServer struct {
	ctx                context.Context
	cancel             context.CancelFunc
	client             *TunnelClient
	address            string
	connectionAttempts int
	logger             logger.Logger
}

func newTunnelServer(client *TunnelClient, address string) *tunnelServer {
	ctx, cancel := context.WithCancel(context.Background())

	return &tunnelServer{
		client:             client,
		address:            address,
		connectionAttempts: 0,
		ctx:                ctx,
		cancel:             cancel,
		logger:             log.WithGroup("tunnel"),
	}
}

func (ts *tunnelServer) ConnectAndServe() {
	go func() {
		ts.logger.Debug("connecting to tunnel server at", "server", ts.address)
		for {
		StartConnectionLoop:

			// Check if the max connection attempts have been reached
			if ts.connectionAttempts >= maxConnectionAttempts {
				ts.logger.Error("maximum connection attempts reached for server , giving up", "server", ts.address)

				// Remove the server from the list of servers
				ts.client.serverListMutex.Lock()
				delete(ts.client.serverList, ts.address)

				// If there's no more servers in the list then exit
				if len(ts.client.serverList) == 0 {
					ts.client.cancel()
				}
				ts.client.serverListMutex.Unlock()

				return
			}

			// Set the target URL
			var url string
			if ts.client.tunnelType == WebTunnel {
				url = ts.address + "/tunnel/server/" + ts.client.tunnelName
			} else {
				url = fmt.Sprintf("%s/tunnel/spaces/%s/%d", ts.address, ts.client.spaceName, ts.client.spacePort)
			}

			// Swap leading http to ws
			url = strings.NewReplacer("http://", "ws://", "https://", "wss://").Replace(url)

			// Open the websocket
			header := http.Header{"Authorization": []string{fmt.Sprintf("Bearer %s", ts.client.token)}}
			dialer := websocket.DefaultDialer
			dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: ts.client.skipTLSVerify}
			dialer.HandshakeTimeout = 5 * time.Second
			ws, response, err := dialer.Dial(url, header)
			if err != nil {
				if response != nil {
					if response.StatusCode == http.StatusUnauthorized {
						log.Fatal("Failed to authenticate with server, check permissions")
					} else if response.StatusCode == http.StatusNotFound {
						if ts.client.tunnelType == WebTunnel {
							log.Fatal("Server does not support tunnels")
						} else {
							log.Fatal("Unable to find space")
						}
					} else if response.StatusCode == http.StatusForbidden {
						log.Fatal("Tunnels are not available on your account")
					} else if response.StatusCode == http.StatusServiceUnavailable {
						log.Fatal("Tunnel limit reached")
					}
				}

				log.WithError(err).Error("Error while opening websocket:")
				time.Sleep(connectRetryDelay)
				ts.connectionAttempts++
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
				log.WithError(err).Error("Creating mux session:")
				ws.Close()
				time.Sleep(connectRetryDelay)
				ts.connectionAttempts++
				goto StartConnectionLoop
			}

			// Loop forever waiting for connections on the mux session
			for {
				select {
				case <-ts.ctx.Done():
					log.Debug("Tunnel server  context cancelled, shutting down connection loop", "server", ts.address)
					muxSession.Close()
					ws.Close()
					return
				default:
					// Accept a new connection
					stream, err := muxSession.Accept()
					if err != nil {
						// In the case of errors, destroy the session and start over
						muxSession.Close()
						ws.Close()

						if ts.client.tunnelType == PortTunnel && err.Error() == "websocket: close 1006 (abnormal closure): unexpected EOF" {
							log.Info("Agent disconnected")
							ts.client.cancel()
							return
						}

						log.WithError(err).Error("Accepting connection:")

						// Wait before trying again
						time.Sleep(connectRetryDelay)
						goto StartConnectionLoop
					}

					go ts.handleTunnelStream(stream)
				}
			}
		}
	}()
}

func (ts *tunnelServer) Shutdown() {
	ts.cancel()
}

func (ts *tunnelServer) handleTunnelStream(stream net.Conn) {
	defer stream.Close()

	// Read the 1st byte to determine if this is a new connection or terminate
	buf := make([]byte, 1)
	_, err := stream.Read(buf)
	if err != nil {
		log.WithError(err).Error("Error reading from stream:")
		return
	}

	// If the byte is 0, then close the stream
	if buf[0] == 0 {
		log.Info("Received tunnel close request from server")
		ts.client.cancel()
		return
	}

	if ts.client.protocol == "http" || ts.client.protocol == "tcp" {
		agent_client.ProxyTcp(stream, fmt.Sprintf("%d", ts.client.localPort))
	} else if ts.client.protocol == "https" || ts.client.protocol == "tls" {
		var tlsName string
		if ts.client.tlsName != "" {
			tlsName = ts.client.tlsName
		} else {
			tlsName = "127.0.0.1"
		}
		agent_client.ProxyTcpTls(stream, fmt.Sprintf("%d", ts.client.localPort), tlsName, ts.client.localPortSkipTLSVerify)
	}
}
