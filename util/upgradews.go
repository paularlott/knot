package util

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

var (
	upgrader websocket.Upgrader
)

// Upgrade the connection to a websocket connection
func UpgradeToWS(w http.ResponseWriter, r *http.Request) *websocket.Conn {
  // If upgrader not initialized then initialize it
  if upgrader.CheckOrigin == nil {
    upgrader = websocket.Upgrader{
      ReadBufferSize:   1024,
      WriteBufferSize:  1024,
      HandshakeTimeout: 10 * time.Second,
      EnableCompression: true,
      CheckOrigin: func(r *http.Request) bool {
        return true
      },
    }
  }

  // Upgrade the connection to a websocket
  ws, err := upgrader.Upgrade(w, r, nil)
  if err != nil {
    w.WriteHeader(http.StatusInternalServerError)
    log.Error().Msgf("Error while upgrading: %s", err)
    return nil
  }

  return ws
}
