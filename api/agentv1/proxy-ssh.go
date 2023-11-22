package agentv1

import (
	"fmt"
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
    log.Error().Msg("proxySSH: error while upgrading to websocket")
    return
  }

  // Open tcp connection to target
  dial := fmt.Sprintf("127.0.0.1:%d", sshPort)
  tcpConn, err := net.DialTimeout("tcp", dial, 10 * time.Second)
  if err != nil {
    ws.Close()
    log.Error().Msgf("proxySSH: error while dialing %s: %s", dial, err.Error())
    return
  }

  copier := proxy.NewCopier(tcpConn, ws)
  go copier.Run()
}
