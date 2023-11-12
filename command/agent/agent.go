package commands_agent

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/paularlott/knot/agent"
	"github.com/paularlott/knot/command"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var codeServerPort string = ""

func init() {
  agentCmd.Flags().IntP("code-server", "", 0, "The port code-server is running on.")
  agentCmd.Flags().IntP("ssh", "", 0, "The port sshd is running on.")
  agentCmd.Flags().BoolP("disable-http", "", false, "If given then disables http proxy.")
  agentCmd.Flags().BoolP("disable-tcp", "", false, "If given then disables tcp proxy.")
  agentCmd.Flags().StringP("listen", "l", "127.0.0.1:3001", "The address and port to listen on.")

  command.RootCmd.AddCommand(agentCmd)
}

var agentCmd = &cobra.Command{
  Use:   "agent",
  Short: "Start the knot agent",
  Long:  `Start the knot agent and listen for incoming connections.

The agent will listen on the port specified by the --listen flag and proxy requests to the code-server instance running on the host.`,
  Args: cobra.NoArgs,
  Run: func(cmd *cobra.Command, args []string) {
    listen := cmd.Flag("listen").Value.String()

    log.Info().Msgf("agent: listening on: %s", listen)

    router := chi.NewRouter()
    router.Mount("/", agent.Routes(cmd))

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
    signal.Notify(c, os.Interrupt)

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
