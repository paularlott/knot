package commands_agent

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/paularlott/knot/agent"
	"github.com/paularlott/knot/api/agentv1"
	"github.com/paularlott/knot/command"
	"github.com/paularlott/knot/util/validate"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
  agentCmd.Flags().StringP("server", "s", "", "The address of the server to connect to.\nOverrides the " + command.CONFIG_ENV_PREFIX + "_SERVER environment variable if set.")
  agentCmd.Flags().StringP("space-id", "", "", "The ID of the space the agent is providing.\nOverrides the " + command.CONFIG_ENV_PREFIX + "_SPACEID environment variable if set.")
  agentCmd.Flags().StringP("nameserver", "n", "", "The nameserver to use for SRV lookups (default use system resolver).\nOverrides the " + command.CONFIG_ENV_PREFIX + "_NAMESERVER environment variable if set.")
  agentCmd.Flags().StringP("listen", "l", "0.0.0.0:3000", "The address and port to listen on.")
  agentCmd.Flags().IntP("code-server-port", "", 0, "The port code-server is running on.\nOverrides the " + command.CONFIG_ENV_PREFIX + "_CODE_SERVER_PORT environment variable if set.")
  agentCmd.Flags().IntP("ssh-port", "", 0, "The port sshd is running on.\nOverrides the " + command.CONFIG_ENV_PREFIX + "_SSH_PORT environment variable if set.")
  agentCmd.Flags().StringSliceP("tcp-port", "p", []string{}, "Can be specified multiple times to give the list of TCP ports to be exposed to the client.\nOverrides the " + command.CONFIG_ENV_PREFIX + "_TCP_PORT environment variable if set.")
  agentCmd.Flags().BoolP("update-authorized-keys", "", true, "If given then the agent will update the authorized_keys file with the SSH public key of the user.\nOverrides the " + command.CONFIG_ENV_PREFIX + "_UPDATE_AUTHORIZED_KEYS environment variable if set.")
  agentCmd.Flags().BoolP("enable-terminal", "", true, "If given then the agent will enable the web terminal.\nOverrides the " + command.CONFIG_ENV_PREFIX + "_ENABLE_TERMINAL environment variable if set.")

  // TODO Add all these to viper and create an agent scaffold example
  agentCmd.Flags().BoolP("disable-http", "", false, "If given then disables http proxy.")

  command.RootCmd.AddCommand(agentCmd)
}

var agentCmd = &cobra.Command{
  Use:   "agent",
  Short: "Start the knot agent",
  Long:  `Start the knot agent and listen for incoming connections.

The agent will listen on the port specified by the --listen flag and proxy requests to the code-server instance running on the host.`,
  Args: cobra.NoArgs,
  PreRun: func(cmd *cobra.Command, args []string) {
    viper.BindPFlag("agent.server", cmd.Flags().Lookup("server"))
    viper.BindEnv("agent.server", command.CONFIG_ENV_PREFIX + "_SERVER")

    viper.BindPFlag("agent.space-id", cmd.Flags().Lookup("space-id"))
    viper.BindEnv("agent.space-id", command.CONFIG_ENV_PREFIX + "_SPACEID")

    viper.BindPFlag("agent.nameserver", cmd.Flags().Lookup("nameserver"))
    viper.BindEnv("agent.nameserver", command.CONFIG_ENV_PREFIX + "_NAMESERVER")
    viper.SetDefault("agent.nameserver", "")

    viper.BindPFlag("agent.listen", cmd.Flags().Lookup("listen"))
    viper.BindEnv("agent.listen", command.CONFIG_ENV_PREFIX + "_LISTEN")
    viper.SetDefault("agent.listen", "0.0.0.0:3000")

    viper.BindPFlag("agent.port.code-server", cmd.Flags().Lookup("code-server-port"))
    viper.BindEnv("agent.port.code-server", command.CONFIG_ENV_PREFIX + "_CODE_SERVER_PORT")
    viper.SetDefault("agent.port.code-server", "0")

    viper.BindPFlag("agent.port.ssh", cmd.Flags().Lookup("ssh-port"))
    viper.BindEnv("agent.port.ssh", command.CONFIG_ENV_PREFIX + "_SSH_PORT")
    viper.SetDefault("agent.port.ssh", "0")

    viper.BindPFlag("agent.port.tcp-port", cmd.Flags().Lookup("tcp-port"))
    viper.BindEnv("agent.port.tcp-port", command.CONFIG_ENV_PREFIX + "_TCP_PORT")

    viper.BindPFlag("agent.update-authorized-keys", cmd.Flags().Lookup("update-authorized-keys"))
    viper.BindEnv("agent.update-authorized-keys", command.CONFIG_ENV_PREFIX + "_UPDATE_AUTHORIZED_KEYS")
    viper.SetDefault("agent.update-authorized-keys", true)

    viper.BindPFlag("agent.enable-terminal", cmd.Flags().Lookup("enable-terminal"))
    viper.BindEnv("agent.enable-terminal", command.CONFIG_ENV_PREFIX + "_ENABLE_TERMINAL")
    viper.SetDefault("agent.enable-terminal", true)
  },
  Run: func(cmd *cobra.Command, args []string) {
    listen := viper.GetString("agent.listen")
    serverAddr := viper.GetString("agent.server")
    nameserver := viper.GetString("agent.nameserver")
    spaceId := viper.GetString("agent.space-id")

    // Build a map of the available tcp ports
    ports := viper.GetStringSlice("agent.port.tcp-port")
    agentv1.AllowedPortMap = make(map[string]bool, len(ports))
    for _, port := range ports {
      agentv1.AllowedPortMap[port] = true
    }

    // Add the ssh port to the map
    sshPort := viper.GetInt("agent.port.ssh")
    if sshPort != 0 {
      agentv1.AllowedPortMap[fmt.Sprintf("%d", sshPort)] = true
    }

    // Check address given and valid URL
    if serverAddr == "" || !validate.Uri(serverAddr) {
      log.Fatal().Msg("server address is required")
    }

    // Check the key is given
    if len(spaceId) != 36 {
      log.Fatal().Msg("space-id is required and must be a valid space ID")
    }

    // Initialize the router API
    router := chi.NewRouter()
    router.Mount("/", agentv1.Routes(cmd))

    // Register the agent with the server
    agent.Register(serverAddr, nameserver, spaceId)

    // Pings the server periodically to keep the agent alive
    go agent.ReportState(serverAddr, nameserver, spaceId, viper.GetInt("agent.port.code-server"), viper.GetInt("agent.port.ssh"))

    log.Info().Msgf("agent: listening on: %s", listen)

    // Run the http server
    server := &http.Server{
      Addr:           listen,
      Handler:        router,
      ReadTimeout:    10 * time.Second,
      WriteTimeout:   10 * time.Second,
    }

    go func() {
      if err := server.ListenAndServe(); err != http.ErrServerClosed {
        log.Fatal().Msg(err.Error())
      }
    }()

    c := make(chan os.Signal, 1)
    signal.Notify(c, os.Interrupt, syscall.SIGTERM)

    // Block until we receive our signal.
    <-c

    ctx, cancel := context.WithTimeout(context.Background(), 30 * time.Second)
    defer cancel()
    server.Shutdown(ctx)
    fmt.Println("\r")
    log.Info().Msg("agent: shutdown")
    os.Exit(0)
  },
}
