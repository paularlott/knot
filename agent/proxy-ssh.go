package agent

import (
	"net"
	"net/http"
	"time"

	"github.com/paularlott/knot/proxy"
	"github.com/paularlott/knot/util"

	"github.com/rs/zerolog/log"
)

func proxySSH(w http.ResponseWriter, r *http.Request) {
  ws := util.UpgradeToWS(w, r);
  if ws == nil {
    log.Error().Msg("Error while upgrading to websocket")
    return
  }

  // Open tcp connection to target
  dial := net.JoinHostPort("127.0.0.1", sshPort)
  tcpConn, err := net.DialTimeout("tcp", dial, 10 * time.Second)
  if err != nil {
    ws.Close()
    log.Error().Msgf("Error while dialing %s: %s", dial, err.Error())
    return
  }

  copier := proxy.NewCopier(tcpConn, ws)
  go copier.Run()
}
