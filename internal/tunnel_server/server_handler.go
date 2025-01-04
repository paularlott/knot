package tunnel_server

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/agentapi/logger"
	"github.com/paularlott/knot/internal/wsconn"
	"github.com/paularlott/knot/util"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/hashicorp/yamux"
	"github.com/rs/zerolog/log"
)

type tunnelSession struct {
	user       *model.User
	tunnelName string
	muxSession *yamux.Session
	ws         *websocket.Conn
}

var (
	tunnelMutex = sync.RWMutex{}
	tunnels     = make(map[string]*tunnelSession)
)

func HandleTunnel(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	// Check the user has permission to create a tunnel
	if !user.HasPermission(model.PermissionUseTunnels) {
		log.Error().Msgf("tunnel: user %s does not have permission to create tunnels", user.Username)
		w.WriteHeader(http.StatusForbidden)
		return
	}

	webName := fmt.Sprintf("%s--%s", user.Username, chi.URLParam(r, "tunnel_name"))

	log.Info().Msgf("tunnel: new tunnel %s", webName)

	// Upgrade to a websocket
	ws := util.UpgradeToWS(w, r)
	if ws == nil {
		log.Error().Msg("tunnel: error while upgrading to websocket")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Create a new tunnel session
	session := &tunnelSession{
		user:       user,
		tunnelName: chi.URLParam(r, "tunnel_name"),
		ws:         ws,
	}

	localConn := wsconn.New(ws)
	var err error

	session.muxSession, err = yamux.Server(localConn, &yamux.Config{
		AcceptBacklog:          256,
		EnableKeepAlive:        true,
		KeepAliveInterval:      30 * time.Second,
		ConnectionWriteTimeout: 10 * time.Second,
		MaxStreamWindowSize:    256 * 1024,
		StreamCloseTimeout:     5 * time.Minute,
		StreamOpenTimeout:      75 * time.Second,
		LogOutput:              nil,
		Logger:                 logger.NewMuxLogger(),
	})
	if err != nil {
		log.Error().Msgf("tunnel: creating mux session: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		ws.Close()
		return
	}

	// Track the connection closing
	go func() {
		<-session.muxSession.CloseChan()

		log.Debug().Msgf("tunnel: detected connection closing %s", webName)

		session.muxSession.Close()
		session.ws.Close()

		tunnelMutex.Lock()
		delete(tunnels, webName)
		tunnelMutex.Unlock()
		log.Info().Msgf("tunnel: closed %s", webName)
	}()

	// Add the tunnel to the map so that traffic can route to it
	tunnelMutex.Lock()
	tunnels[webName] = session
	tunnelMutex.Unlock()
}
