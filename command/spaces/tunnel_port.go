package command_spaces

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
	"github.com/spf13/viper"
)

func init() {
	tunnelPortCmd.PersistentFlags().BoolP("tls", "", true, "Enable TLS encryption for the tunnel.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_TUNNEL_TLS environment variable if set.")
}

var tunnelPortCmd = &cobra.Command{
	Use:   "tunnel <space> <listen> <port> [flags]",
	Short: "Open a tunnel",
	Long: `Open a tunnel between a port inside a space and a port on the local machine.

  space     The name of the space to connect to e.g. test1
  listen    The port to listen on within the space e.g. 80
  port      The local port to connect to e.g. 8080`,
	Args: cobra.ExactArgs(3),
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		viper.BindPFlag("tls", cmd.Flags().Lookup("tls"))
		viper.BindEnv("tls", config.CONFIG_ENV_PREFIX+"_TUNNEL_TLS")
		viper.SetDefault("tls", true)
	},
	Run: func(cmd *cobra.Command, args []string) {
		alias, _ := cmd.Flags().GetString("alias")
		cfg := config.GetServerAddr(alias)

		spaceName := args[0]

		listenPort, err := strconv.Atoi(args[1])
		if err != nil || listenPort < 1 || listenPort > 65535 {
			cobra.CheckErr("Invalid port number, port numbers must be between 1 and 65535")
		}

		localPort, err := strconv.Atoi(args[2])
		if err != nil || localPort < 1 || localPort > 65535 {
			cobra.CheckErr("Invalid port number, port numbers must be between 1 and 65535")
		}

		opts := tunnel_server.TunnelOpts{
			Type:      tunnel_server.PortTunnel,
			Protocol:  "tcp",
			LocalPort: uint16(localPort),
			SpaceName: spaceName,
			SpacePort: uint16(listenPort),
		}

		if viper.GetBool("tls") {
			opts.Protocol = "tls"
		}

		client := tunnel_server.NewTunnelClient(
			cfg.WsServer,
			cfg.HttpServer,
			cfg.ApiToken,
			&opts,
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
