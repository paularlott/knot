package cmd

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/paularlott/knot/proxy"
	"github.com/paularlott/knot/web"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	upgrader websocket.Upgrader
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

    // Websocket upgrader
    upgrader = websocket.Upgrader{
      ReadBufferSize:   1024,
      WriteBufferSize:  1024,
      HandshakeTimeout: 10 * time.Second,
      CheckOrigin: func(r *http.Request) bool {
        return true
      },
    }

    log.Println("Starting server on", listen)

    router := mux.NewRouter()

    // Define endpoints
    router.HandleFunc("/forward-port/{host}/{port:\\d+}", func(w http.ResponseWriter, r *http.Request) {
      if ws := upgradeToWS(w, r); ws != nil {
        proxy.HandleWSProxyServer(w, r, ws, viper.GetString("nameserver"))
      }
    })
    router.HandleFunc("/ping", web.HandlePing)
    router.HandleFunc("/lookup/{service}", web.HandleLookup)

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
        log.Println(err)
      }
    }()

    c := make(chan os.Signal, 1)
    signal.Notify(c, os.Interrupt)

    // Block until we receive our signal.
    <-c

    ctx, cancel := context.WithTimeout(context.Background(), 30 * time.Second)
    defer cancel()
    server.Shutdown(ctx)
    log.Println("Shut down")
    os.Exit(0)
  },
}

// Upgrade the connection to a websocket connection
func upgradeToWS(w http.ResponseWriter, r *http.Request) *websocket.Conn {
  // Upgrade the connection to a websocket
  ws, err := upgrader.Upgrade(w, r, nil)
  if err != nil {
    w.WriteHeader(http.StatusInternalServerError)
    log.Printf("Error while upgrading: %s", err)
    return nil
  }

  return ws
}
