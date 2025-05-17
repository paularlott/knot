package command_tunnel

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/paularlott/knot/internal/agentapi/agent_client"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/tunnel_server"
	"github.com/paularlott/knot/util/validate"
	"github.com/rs/zerolog/log"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	TunnelCmd.PersistentFlags().StringP("server", "s", "", "The address of the remote server to create the tunnel on.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_SERVER environment variable if set.")
	TunnelCmd.PersistentFlags().StringP("token", "t", "", "The token to use for authentication.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_TOKEN environment variable if set.")
	TunnelCmd.PersistentFlags().BoolP("tls-skip-verify", "", true, "Skip TLS verification when talking to server.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_TLS_SKIP_VERIFY environment variable if set.")
	TunnelCmd.Flags().StringP("alias", "a", "default", "The server alias to use.")

	TunnelCmd.Flags().StringP("hostname", "n", "", "The hostname to present when using https protocol.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_HOSTNAME environment variable if set.")
}

var TunnelCmd = &cobra.Command{
	Use: `tunnel <protocol> <port> <name>

  protocol      The type of endpoint, either http or https.
  port          The local port to tunnel to.
  name          The name of the tunnel.
`,
	Short: "Manage a tunnel",
	Long: `The tunnel command allows you to create a tunnel to expose a local port on the internet.

The tunnel can be created to expose either an http or https endpoint, the name provided is prepended with the username e.g. <user>--<tunnel_name>.<domain>.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		alias, _ := cmd.Flags().GetString("alias")

		viper.BindPFlag("client."+alias+".server", cmd.PersistentFlags().Lookup("server"))
		viper.BindEnv("client."+alias+".server", config.CONFIG_ENV_PREFIX+"_SERVER")

		viper.BindPFlag("client."+alias+".token", cmd.PersistentFlags().Lookup("token"))
		viper.BindEnv("client."+alias+".token", config.CONFIG_ENV_PREFIX+"_TOKEN")

		viper.BindPFlag("tls_skip_verify", cmd.Flags().Lookup("tls-skip-verify"))
		viper.BindEnv("tls_skip_verify", config.CONFIG_ENV_PREFIX+"_TLS_SKIP_VERIFY")
		viper.SetDefault("tls_skip_verify", true)
	},
	Args: cobra.ExactArgs(3),
	Run: func(cmd *cobra.Command, args []string) {

		// Validate the protocol
		if args[0] != "http" && args[0] != "https" {
			cobra.CheckErr("Invalid protocol type, must be either http or https")
		}

		// Validate that the port is a number
		port, err := strconv.Atoi(args[1])
		if err != nil || port < 1 || port > 65535 {
			cobra.CheckErr("Invalid port number, port numbers must be between 1 and 65535")
		}

		// Validate the name is all lowercase and only contains letters, numbers and dashes
		if !validate.Name(args[2]) {
			cobra.CheckErr("Invalid name, must be all lowercase and only contain letters, numbers and dashes")
		}

		alias, _ := cmd.Flags().GetString("alias")
		cfg := config.GetServerAddr(alias)
		tunnel_server.ConnectAndForward(cfg.WsServer, cfg.HttpServer, cfg.ApiToken, args[0], uint16(port), args[2], cmd.Flag("hostname").Value.String())

		// Wait for ctrl-c
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)

		// Block until we receive our signal.
		<-c

		agent_client.Shutdown()
		fmt.Println("\r")
		log.Info().Msg("Tunnel shutdown")
		os.Exit(0)
	},
}
