package commands_agent

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/paularlott/knot/command"
	"github.com/paularlott/knot/proxy"
	"github.com/paularlott/knot/util"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var codeServerPort string = ""

func init() {
  agentCmd.Flags().StringP("code-server", "", "", "The local port of the code-server instance.")
  agentCmd.Flags().StringP("listen", "l", "3001", "The address to listen on.")

  command.RootCmd.AddCommand(agentCmd)
}

var agentCmd = &cobra.Command{
  Use:   "agent",
  Short: "Start the knot agent",
  Long:  `Start the knot agent and listen for incoming connections.

The agent will listen on the port specified by the --listen flag and proxy requests to the code-server instance running on the host.`,
  Args: cobra.NoArgs,
  Run: func(cmd *cobra.Command, args []string) {
    listenPort := cmd.Flag("listen").Value.String()
    codeServerPort = cmd.Flag("code-server").Value.String()

    log.Info().Msgf("Starting agent, listening on port: %s", listenPort)

    router := chi.NewRouter()

    // If code server port given the enable the porxy
    if codeServerPort != "" {
      log.Info().Msgf("Proxying code-server on port: %s", codeServerPort)
      router.HandleFunc("/code-server/*", proxyCodeServer);
    }

    log.Info().Msgf("Proxying SSH on port: %s", "2222")
    router.HandleFunc("/ssh/*", proxySSH)

    log.Info().Msgf("Proxying Web on port: %s", "9000")
    router.HandleFunc("/web/*", proxyWeb)

    // Run the http server
    server := &http.Server{
      Addr:           "127.0.0.1:" + listenPort,
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
    log.Info().Msg("Agent Shutdown")
    os.Exit(0)
  },
}

func proxyCodeServer(w http.ResponseWriter, r *http.Request) {
  target, _ := url.Parse("http://127.0.0.1:" + codeServerPort)
  proxy := httputil.NewSingleHostReverseProxy(target)

  r.URL.Path = strings.TrimPrefix(r.URL.Path, "/code-server")

  proxy.ServeHTTP(w, r)
}

func proxyWeb(w http.ResponseWriter, r *http.Request) {
  target, _ := url.Parse("http://127.0.0.1:" + "9000")
  proxy := httputil.NewSingleHostReverseProxy(target)

  r.URL.Path = strings.TrimPrefix(r.URL.Path, "/web")

  proxy.ServeHTTP(w, r)
}

func proxySSH(w http.ResponseWriter, r *http.Request) {
  ws := util.UpgradeToWS(w, r);
  if ws == nil {
    log.Error().Msg("Error while upgrading to websocket")
    return
  }

  host := "127.0.0.1"
  port := "2222"

  log.Info().Msgf("Proxying to SSH port: %s", port)

  // Open tcp connection to target
  tcpConn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 10 * time.Second)
  if err != nil {
    ws.Close()
    log.Error().Msgf("Error while dialing %s:%s: %s", host, port, err.Error())
    return
  }

  copier := proxy.NewCopier(tcpConn, ws)
  go copier.Run()
}
