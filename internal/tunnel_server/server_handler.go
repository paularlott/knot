package tunnel_server

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/paularlott/knot/internal/agentapi/logger"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/wsconn"
	"github.com/paularlott/knot/util"
	"github.com/paularlott/knot/util/validate"

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
	var err error

	user := r.Context().Value("user").(*model.User)

	// Check the user has permission to create a tunnel
	if !user.HasPermission(model.PermissionUseTunnels) {
		log.Error().Msgf("tunnel: user %s does not have permission to create tunnels", user.Username)
		w.WriteHeader(http.StatusForbidden)
		return
	}

	// Calculate the number of tunnels allowed
	db := database.GetInstance()

	// Get the groups and build a map
	groups, err := db.GetGroups()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	groupMap := make(map[string]*model.Group)
	for _, group := range groups {
		groupMap[group.Id] = group
	}

	maxTunnels := user.MaxTunnels
	for _, group := range user.Groups {
		if g, ok := groupMap[group]; ok {
			maxTunnels += g.MaxTunnels
		}
	}

	// It a tunnel limit is set then check user has not exceeded it
	if maxTunnels > 0 && CountUserTunnels(user.Id) >= maxTunnels {
		log.Error().Msgf("tunnel: user %s has exceeded the maximum number of tunnels", user.Username)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	tunnelName := r.PathValue("tunnel_name")
	if !validate.Subdomain(tunnelName) {
		log.Error().Msgf("tunnel: invalid tunnel name %s", tunnelName)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	tunnelName = strings.ToLower(tunnelName)
	webName := strings.ToLower(fmt.Sprintf("%s--%s", user.Username, tunnelName))

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

func CountUserTunnels(userId string) uint32 {
	var count uint32

	tunnelMutex.RLock()
	defer tunnelMutex.RUnlock()

	for _, t := range tunnels {
		if t.user.Id == userId {
			count++
		}
	}

	return count
}

func GetTunnelsForUser(userId string) []string {
	var userTunnels []string

	tunnelMutex.RLock()
	defer tunnelMutex.RUnlock()

	for _, t := range tunnels {
		if t.user.Id == userId {
			userTunnels = append(userTunnels, t.tunnelName)
		}
	}

	return userTunnels
}

func DeleteTunnel(userId, tunnelName string) error {

	// Split on -- and take the last part
	parts := strings.Split(tunnelName, "--")
	if len(parts) != 2 {
		return fmt.Errorf("invalid tunnel name format")
	}
	tunnelName = parts[1]

	tunnelMutex.Lock()
	defer tunnelMutex.Unlock()

	fmt.Println("deleteing tunnel", userId, tunnelName)

	for key, t := range tunnels {
		fmt.Println("checking tunnel", t.user.Id, t.tunnelName)
		if t.user.Id == userId && t.tunnelName == tunnelName {
			// Open a new stream to the tunnel client
			stream, err := t.muxSession.Open()
			if err == nil {
				defer stream.Close()

				// Write a byte with a value of 0 so the client knows to close the stream
				stream.Write([]byte{0})

				// Wait for the client to close the stream
				time.Sleep(1 * time.Second)
			}

			t.muxSession.Close()
			t.ws.Close()
			delete(tunnels, key)
			return nil
		}
	}

	return fmt.Errorf("tunnel not found")
}
