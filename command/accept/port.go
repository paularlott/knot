package commands_accept

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/tunnel_server"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var portCmd = &cobra.Command{
	Use:   "port <listen> <space> <port> [flags]",
	Short: "Accept a connection to a port",
	Long: `Accepts a connection from a space to a local port.

  listen    The port to listen on within the space e.g. :80
  space     The name of the space to connect to e.g. test1
  port      The local port to connect to e.g. 8080`,
	Args: cobra.ExactArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		alias, _ := cmd.Flags().GetString("alias")
		cfg := config.GetServerAddr(alias)

		listenPort, err := strconv.Atoi(args[0])
		if err != nil || listenPort < 1 || listenPort > 65535 {
			cobra.CheckErr("Invalid port number, port numbers must be between 1 and 65535")
		}

		localPort, err := strconv.Atoi(args[2])
		if err != nil || localPort < 1 || localPort > 65535 {
			cobra.CheckErr("Invalid port number, port numbers must be between 1 and 65535")
		}

		client := tunnel_server.NewTunnelClient(
			cfg.WsServer,
			cfg.HttpServer,
			cfg.ApiToken,
			&tunnel_server.TunnelOpts{
				Type:      tunnel_server.PortTunnel,
				Protocol:  "tcp",
				LocalPort: uint16(localPort),
				SpaceName: args[1],
				SpacePort: uint16(listenPort),
			},
		)
		if err := client.ConnectAndServe(); err != nil {
			log.Fatal().Err(err).Msg("Failed to create tunnel")
		}

		// Wait for ctrl-c
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)

		// Block until we receive ctrl-c or tunnel context is cancelled
		select {
		case <-client.GetCtx().Done():
		case <-c:
		}

		client.Shutdown()

		fmt.Println("\r")
		log.Info().Msg("Tunnel shutdown")
		os.Exit(0)
	},
}
