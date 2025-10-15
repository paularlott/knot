package tunnel_server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/paularlott/knot/internal/agentapi/logger"
	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util"
	"github.com/paularlott/knot/internal/wsconn"

	"github.com/hashicorp/yamux"
	"github.com/paularlott/knot/internal/log"
)

func HandleCreatePortTunnel(w http.ResponseWriter, r *http.Request, muxSession *yamux.Session, agentSessionId string, port uint16, space *model.Space, user *model.User) error {
	var err error

	tunnelName := fmt.Sprintf("--%s:%d", agentSessionId, port)

	log.Info("tunnel: new tunnel :", "tunnel_name", space.Name)

	// Upgrade to a websocket
	ws := util.UpgradeToWS(w, r)
	if ws == nil {
		log.Error("tunnel: error while upgrading to websocket")
		w.WriteHeader(http.StatusInternalServerError)
		return fmt.Errorf("error while upgrading to websocket")
	}

	// Create a new tunnel session
	session := &tunnelSession{
		tunnelType: PortTunnel,
		user:       user,
		tunnelName: tunnelName,
		ws:         ws,
	}

	localConn := wsconn.New(ws)

	session.muxSession, err = yamux.Server(localConn, &yamux.Config{
		AcceptBacklog:          256,
		EnableKeepAlive:        true,
		KeepAliveInterval:      2 * time.Second,
		ConnectionWriteTimeout: 10 * time.Second,
		MaxStreamWindowSize:    256 * 1024,
		StreamCloseTimeout:     3 * time.Minute,
		StreamOpenTimeout:      3 * time.Second,
		LogOutput:              nil,
		Logger:                 logger.NewMuxLogger(),
	})
	if err != nil {
		log.WithError(err).Error("tunnel: creating mux session:")
		w.WriteHeader(http.StatusInternalServerError)
		ws.Close()
		return err
	}

	// Add the tunnel to the map so that traffic can route to it
	tunnelMutex.Lock()
	tunnels[tunnelName] = session
	tunnelMutex.Unlock()

	defer func() {
		log.Debug("tunnel: detected connection closing :", "tunnel_name", space.Name)

		session.muxSession.Close()
		session.ws.Close()
		localConn.Close()

		tunnelMutex.Lock()
		delete(tunnels, tunnelName)
		tunnelMutex.Unlock()
		log.Info("tunnel: closed :", "tunnel_name", space.Name)
	}()

	// Open a new stream to the agent, we hold the stream open to keep the session locked to this server
	stream, err := muxSession.Open()
	if err != nil {
		log.WithError(err).Debug("Error opening stream to agent")
		w.WriteHeader(http.StatusInternalServerError)
		return err
	}
	defer stream.Close()

	// Tell the agent about the new tunnel
	if err := msg.WriteCommand(stream, msg.CmdTunnelPort); err != nil {
		log.WithError(err).Debug("Error writing command")
		w.WriteHeader(http.StatusInternalServerError)
		return err
	}
	if err := msg.WriteMessage(stream, &msg.TcpPort{
		Port: port,
	}); err != nil {
		log.WithError(err).Debug("Error writing message")
		w.WriteHeader(http.StatusInternalServerError)
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		// Reading from stream until EOF or error indicates the stream has closed
		buf := make([]byte, 1)
		_, err := stream.Read(buf)
		if err != nil {
			log.WithError(err).Debug("tunnel: control stream closed:")
			session.muxSession.Close()
			cancel()
		}
	}()

	// Wait for the client to disconnect
	select {
	case <-session.muxSession.CloseChan():
	case <-ctx.Done():
	}

	return nil
}
