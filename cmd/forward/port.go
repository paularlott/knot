package cmd_forward

import (
	"io"
	"net"
	"strconv"

	"github.com/paularlott/knot/util"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var portCmd = &cobra.Command{
  Use:   "port <listen> <service> <port> [flags]",
  Short: "Forward a port direct to the service",
  Long:  `Forwards a local port to a remote server and port via a direct connection.

If <port> is not given then the remote port is found via a DNS SRV lookup against the service name.

  listen    The local port to listen on e.g. :8080
  service   The name of the remote service to connect to e.g. web.service.consul
  port      The optional remote port to connect to e.g. 80`,
  Args: cobra.RangeArgs(2, 3),
  Run: func(cmd *cobra.Command, args []string) {
    var host string
    var port string
    var err error

    listen := args[0]
    service := args[1]

    if len(args) == 3 {
      portInt, err := strconv.Atoi(args[2])
      port = strconv.Itoa(portInt)
      if err != nil || portInt < 1 || portInt > 65535 {
        cobra.CheckErr("Invalid port number, port numbers must be between 1 and 65535")
      }

      host, err = util.GetIP(service, viper.GetString("nameserver"))
    } else {
      host, port, err = util.GetTargetFromSRV(service, viper.GetString("nameserver"))
    }

    if err != nil {
      cobra.CheckErr("Failed to find service")
    }

    log.Info().Msgf("Listening on %s", listen)
    log.Info().Msgf("Forwarding ro %s (%s:%s)", args[0], host, port)

    listener, err := net.Listen("tcp", listen)
    if err != nil {
      log.Fatal().Msgf("Error while opening local port: %s", err.Error())
    }
    defer listener.Close()

    for {
      localConn, err := listener.Accept()
      if err != nil {
        log.Error().Msgf("Could not accept the connection: %s", err.Error())
      }

      go func() {
        remoteConn, err := net.Dial("tcp", net.JoinHostPort(host, port))
        if err != nil {
          localConn.Close()
          log.Fatal().Msg("Can't connect to remote")
        }
        defer remoteConn.Close()

        go func() { io.Copy(localConn, remoteConn) }()
        _, err = io.Copy(remoteConn, localConn);
        if err != nil {
          localConn.Close()
          remoteConn.Close()
          log.Fatal().Msg("Lost connection to remote")
        }
      }()
    }
  },
}
