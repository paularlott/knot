package tunnel_server

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

	"github.com/paularlott/knot/internal/agentapi/agentproxy"
	"github.com/paularlott/knot/internal/wsconn"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/yamux"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/logger"
)

const (
	// connectRetryDelay is the delay before the first reconnect attempt; it
	// doubles per consecutive failed attempt up to maxConnectRetryDelay and
	// resets once connected. Tunnels retry forever, so a knot server restart
	// (or any outage shorter than the agent's lifetime) reforms the tunnel
	// instead of killing it.
	connectRetryDelay    = 1 * time.Second
	maxConnectRetryDelay = 30 * time.Second
)

type tunnelServer struct {
	ctx     context.Context
	cancel  context.CancelFunc
	client  *TunnelClient
	address string
	logger  logger.Logger

	// Active connection state, protected by connMu. Shutdown() closes these
	// directly because the serve goroutine is normally blocked in
	// muxSession.Accept(), which a context cancel cannot interrupt.
	connMu     sync.Mutex
	ws         *websocket.Conn
	muxSession *yamux.Session
}

func newTunnelServer(client *TunnelClient, address string) *tunnelServer {
	// Derive from the client's context: cancelling the client (shutdown, or
	// the server asking the tunnel to close) must stop every connection
	// loop. A loop with its own background context would keep reconnecting
	// after the tunnel was stopped — the tunnel would stay alive while its
	// registry entry is gone.
	ctx, cancel := context.WithCancel(client.ctx)

	return &tunnelServer{
		client:  client,
		address: address,
		ctx:     ctx,
		cancel:  cancel,
		logger:  log.WithGroup("tunnel"),
	}
}

func (ts *tunnelServer) ConnectAndServe() {
	go func() {
		ts.logger.Debug("connecting to tunnel server at", "server", ts.address)
		retryDelay := connectRetryDelay
		for {
		StartConnectionLoop:

			// If shutting down, don't (re)connect.
			select {
			case <-ts.ctx.Done():
				return
			default:
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
				time.Sleep(retryDelay)
				retryDelay = min(retryDelay*2, maxConnectRetryDelay)
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
				time.Sleep(retryDelay)
				retryDelay = min(retryDelay*2, maxConnectRetryDelay)
				goto StartConnectionLoop
			}

			// Track the live connection so Shutdown() can close it.
			ts.connMu.Lock()
			ts.ws = ws
			ts.muxSession = muxSession
			ts.connMu.Unlock()

			// Connected: reset the backoff so the next drop retries promptly.
			retryDelay = connectRetryDelay

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

	// Close the live connection directly. The serve goroutine is normally
	// blocked in muxSession.Accept(); closing the mux session (which closes the
	// underlying websocket) unblocks it and lets the server detect the
	// disconnect so it cleans up the tunnel session.
	ts.connMu.Lock()
	if ts.muxSession != nil {
		ts.muxSession.Close()
	} else if ts.ws != nil {
		ts.ws.Close()
	}
	ts.connMu.Unlock()
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
		agentproxy.ProxyTcp(stream, fmt.Sprintf("%d", ts.client.localPort))
	} else if ts.client.protocol == "https" || ts.client.protocol == "tls" {
		var tlsName string
		if ts.client.tlsName != "" {
			tlsName = ts.client.tlsName
		} else {
			tlsName = "127.0.0.1"
		}
		agentproxy.ProxyTcpTls(stream, fmt.Sprintf("%d", ts.client.localPort), tlsName, ts.client.localPortSkipTLSVerify)
	}
}
