package cmd

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/paularlott/knot/proxy"
	"github.com/paularlott/knot/web"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
  serverCmd.Flags().StringP("listen", "l", "", "The address to listen on (default \"127.0.0.1:3000\").\nOverrides the " + CONFIG_ENV_PREFIX + "_LISTEN environment variable if set.")
  serverCmd.Flags().StringP("nameserver", "n", "", "The nameserver to use for SRV lookups (default use system resolver).\nOverrides the " + CONFIG_ENV_PREFIX + "_NAMESERVER environment variable if set.")

  RootCmd.AddCommand(serverCmd)
}

var serverCmd = &cobra.Command{
  Use:   "server",
  Short: "Start the knot server",
  Long:  `Start the knot server and listen for incoming connections.`,
  Args: cobra.NoArgs,
  PreRun: func(cmd *cobra.Command, args []string) {
    viper.BindPFlag("server.listen", cmd.Flags().Lookup("listen"))
    viper.BindEnv("server.listen", CONFIG_ENV_PREFIX + "_LISTEN")
    viper.SetDefault("server.listen", "127.0.0.1:3000")

    viper.BindPFlag("nameserver", cmd.Flags().Lookup("nameserver"))
    viper.BindEnv("nameserver", CONFIG_ENV_PREFIX + "_NAMESERVER")
    viper.SetDefault("nameserver", "")
  },
  Run: func(cmd *cobra.Command, args []string) {
    listen := viper.GetString("server.listen")

    log.Info().Msgf("Starting server on: %s", listen)

    router := chi.NewRouter()

    // Define endpoints
    router.Get("/forward-port/{host}/{port:\\d+}", proxy.HandleWSProxyServer)
    router.Get("/ping", web.HandlePing)
    router.Get("/lookup/{service}", web.HandleLookup)

    // Run the http server
    server := &http.Server{
      Addr:           listen,
      Handler:        router,
      ReadTimeout:    10 * time.Second,
      WriteTimeout:   10 * time.Second,
      MaxHeaderBytes: 1 << 20,
    }

    go func() {
      if err := server.ListenAndServe(); err != nil {
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
    log.Info().Msg("Server Shutdown")
    os.Exit(0)
  },
}
