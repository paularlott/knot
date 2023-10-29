package command

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"time"

	api_v1 "github.com/paularlott/knot/api/v1"
	"github.com/paularlott/knot/proxy"

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

    router.Mount("/api/v1", api_v1.ApiRoutes())
    router.Mount("/proxy", proxy.Routes())


    router.HandleFunc("/{box}/code-server/*", proxyCodeServer);
    router.Get("/{box}/ssh/*", proxySSH);


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
    log.Info().Msg("Server Shutdown")
    os.Exit(0)
  },
}

func proxyCodeServer(w http.ResponseWriter, r *http.Request) {

  // TODO Change this to look up the IP + Port from consul / DNS
  target, _ := url.Parse("http://127.0.0.1:3001/code-server/")
  proxy := httputil.NewSingleHostReverseProxy(target)

  r.URL.Path = strings.TrimPrefix(r.URL.Path, "/code-server")

  proxy.ServeHTTP(w, r)
}

func proxySSH(w http.ResponseWriter, r *http.Request) {
    log.Info().Msg("proxySSH")

  // TODO Change this to look up the IP + Port from consul / DNS
  target, _ := url.Parse("http://127.0.0.1:3001/ssh/")
  proxy := httputil.NewSingleHostReverseProxy(target)

  r.URL.Path = strings.TrimPrefix(r.URL.Path, "/ssh")

  proxy.ServeHTTP(w, r)
}
