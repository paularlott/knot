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
	"syscall"
	"time"

	"github.com/paularlott/knot/api/apiv1"
	"github.com/paularlott/knot/middleware"
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
  serverCmd.Flags().StringP("url", "u", "", "The URL to use for the server (default \"http://127.0.0.1:3000\").\nOverrides the " + CONFIG_ENV_PREFIX + "_URL environment variable if set.")

  // MySQL
  serverCmd.Flags().BoolP("mysql-enabled", "", false, "Enable MySQL database backend.\nOverrides the " + CONFIG_ENV_PREFIX + "_MYSQL_ENABLED environment variable if set.")
  serverCmd.Flags().StringP("mysql-host", "", "", "The MySQL host to connect to (default \"localhost\").\nOverrides the " + CONFIG_ENV_PREFIX + "_MYSQL_HOST environment variable if set.")
  serverCmd.Flags().IntP("mysql-port", "", 3306, "The MySQL port to connect to (default \"3306\").\nOverrides the " + CONFIG_ENV_PREFIX + "_MYSQL_PORT environment variable if set.")
  serverCmd.Flags().StringP("mysql-user", "", "root", "The MySQL user to connect as (default \"root\").\nOverrides the " + CONFIG_ENV_PREFIX + "_MYSQL_USER environment variable if set.")
  serverCmd.Flags().StringP("mysql-password", "", "", "The MySQL password to use.\nOverrides the " + CONFIG_ENV_PREFIX + "_MYSQL_PASSWORD environment variable if set.")
  serverCmd.Flags().StringP("mysql-database", "", "knot", "The MySQL database to use (default \"knot\").\nOverrides the " + CONFIG_ENV_PREFIX + "_MYSQL_DATABASE environment variable if set.")
  serverCmd.Flags().IntP("mysql-connection-max-idle", "", 2, "The maximum number of idle connections in the connection pool (default \"10\").\nOverrides the " + CONFIG_ENV_PREFIX + "_MYSQL_CONNECTION_MAX_IDLE environment variable if set.")
  serverCmd.Flags().IntP("mysql-connection-max-open", "", 10, "The maximum number of open connections to the database (default \"10\").\nOverrides the " + CONFIG_ENV_PREFIX + "_MYSQL_CONNECTION_MAX_OPEN environment variable if set.")
  serverCmd.Flags().IntP("mysql-connection-max-lifetime", "", 5, "The maximum amount of time in minutes a connection may be reused (default \"5\").\nOverrides the " + CONFIG_ENV_PREFIX + "_MYSQL_CONNECTION_MAX_LIFETIME environment variable if set.")

  // BadgerDB
  serverCmd.Flags().BoolP("badgerdb-enabled", "", false, "Enable BadgerDB database backend.\nOverrides the " + CONFIG_ENV_PREFIX + "_BADGERDB_ENABLED environment variable if set.")
  serverCmd.Flags().StringP("badgerdb-path", "", "./badger", "The path to the BadgerDB database (default \"./badger\").\nOverrides the " + CONFIG_ENV_PREFIX + "_BADGERDB_PATH environment variable if set.")

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

    viper.BindPFlag("server.url", cmd.Flags().Lookup("url"))
    viper.BindEnv("server.url", CONFIG_ENV_PREFIX + "_URL")
    viper.SetDefault("server.url", "http://127.0.0.1:3000")

    viper.BindPFlag("nameserver", cmd.Flags().Lookup("nameserver"))
    viper.BindEnv("nameserver", CONFIG_ENV_PREFIX + "_NAMESERVER")
    viper.SetDefault("nameserver", "")

    // MySQL
    viper.BindPFlag("server.mysql.enabled", cmd.Flags().Lookup("mysql-enabled"))
    viper.BindEnv("server.mysql.enabled", CONFIG_ENV_PREFIX + "_MYSQL_ENABLED")
    viper.SetDefault("server.mysql.enabled", false)
    viper.BindPFlag("server.mysql.host", cmd.Flags().Lookup("mysql-host"))
    viper.BindEnv("server.mysql.host", CONFIG_ENV_PREFIX + "_MYSQL_HOST")
    viper.SetDefault("server.mysql.host", "localhost")
    viper.BindPFlag("server.mysql.port", cmd.Flags().Lookup("mysql-port"))
    viper.BindEnv("server.mysql.port", CONFIG_ENV_PREFIX + "_MYSQL_PORT")
    viper.SetDefault("server.mysql.port", 3306)
    viper.BindPFlag("server.mysql.user", cmd.Flags().Lookup("mysql-user"))
    viper.BindEnv("server.mysql.user", CONFIG_ENV_PREFIX + "_MYSQL_USER")
    viper.SetDefault("server.mysql.user", "root")
    viper.BindPFlag("server.mysql.password", cmd.Flags().Lookup("mysql-password"))
    viper.BindEnv("server.mysql.password", CONFIG_ENV_PREFIX + "_MYSQL_PASSWORD")
    viper.SetDefault("server.mysql.password", "")
    viper.BindPFlag("server.mysql.database", cmd.Flags().Lookup("mysql-database"))
    viper.BindEnv("server.mysql.database", CONFIG_ENV_PREFIX + "_MYSQL_DATABASE")
    viper.SetDefault("server.mysql.database", "knot")
    viper.BindPFlag("server.mysql.connection_max_idle", cmd.Flags().Lookup("mysql-connection-max-idle"))
    viper.BindEnv("server.mysql.connection_max_idle", CONFIG_ENV_PREFIX + "_MYSQL_CONNECTION_MAX_IDLE")
    viper.SetDefault("server.mysql.connection_max_idle", 2)
    viper.BindPFlag("server.mysql.connection_max_open", cmd.Flags().Lookup("mysql-connection-max-open"))
    viper.BindEnv("server.mysql.connection_max_open", CONFIG_ENV_PREFIX + "_MYSQL_CONNECTION_MAX_OPEN")
    viper.SetDefault("server.mysql.connection_max_open", 10)
    viper.BindPFlag("server.mysql.connection_max_lifetime", cmd.Flags().Lookup("mysql-connection-max-lifetime"))
    viper.BindEnv("server.mysql.connection_max_lifetime", CONFIG_ENV_PREFIX + "_MYSQL_CONNECTION_MAX_LIFETIME")
    viper.SetDefault("server.mysql.connection_max_lifetime", 5)

    // BadgerDB
    viper.BindPFlag("server.badgerdb.enabled", cmd.Flags().Lookup("badgerdb-enabled"))
    viper.BindEnv("server.badgerdb.enabled", CONFIG_ENV_PREFIX + "_BADGERDB_ENABLED")
    viper.SetDefault("server.badgerdb.enabled", false)
    viper.BindPFlag("server.badgerdb.path", cmd.Flags().Lookup("badgerdb-path"))
    viper.BindEnv("server.badgerdb.path", CONFIG_ENV_PREFIX + "_BADGERDB_PATH")
    viper.SetDefault("server.badgerdb.path", "./badger")
  },
  Run: func(cmd *cobra.Command, args []string) {
    listen := viper.GetString("server.listen")

    log.Info().Msgf("server: starting on: %s", listen)

    // Initialize the middleware, test if users are present
    middleware.Initialize()

    router := chi.NewRouter()

    router.Mount("/api/v1", apiv1.ApiRoutes())
    router.Mount("/proxy", proxy.Routes())
    router.Mount("/", web.Routes())

// TODO /proxy/agent/{user}/{box}/code-server
// TODO /proxy/agent/{user}/{box}/ssh
// TODO /proxy/agent/{user}/{box}/port/{port}
    router.HandleFunc("/{box}/code-server/*", proxyCodeServer);
    router.HandleFunc("/{box}/port/{port}", proxyTCP);
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
    signal.Notify(c, os.Interrupt, syscall.SIGTERM)

    // Block until we receive our signal.
    <-c

    ctx, cancel := context.WithTimeout(context.Background(), 30 * time.Second)
    defer cancel()
    server.Shutdown(ctx)
    fmt.Println("\r")
    log.Info().Msg("server: shutdown")
    os.Exit(0)
  },
}

func proxyCodeServer(w http.ResponseWriter, r *http.Request) {

  // TODO Change this to look up the IP + Port from consul / DNS
  target, _ := url.Parse("http://127.0.0.1:3001/code-server/")
  proxy := httputil.NewSingleHostReverseProxy(target)

  box := chi.URLParam(r, "box")

  r.URL.Path = strings.TrimPrefix(r.URL.Path, fmt.Sprintf("/%s/code-server", box))

  proxy.ServeHTTP(w, r)
}

func proxySSH(w http.ResponseWriter, r *http.Request) {
  // TODO Change this to look up the IP + Port from consul / DNS
  target, _ := url.Parse("http://127.0.0.1:3001/ssh/")
  proxy := httputil.NewSingleHostReverseProxy(target)

  box := chi.URLParam(r, "box")

  r.URL.Path = strings.TrimPrefix(r.URL.Path, fmt.Sprintf("/%s/ssh", box))

  proxy.ServeHTTP(w, r)
}


func proxyTCP(w http.ResponseWriter, r *http.Request) {
  port := chi.URLParam(r, "port")

  // TODO Change this to look up the IP + Port from consul / DNS
  target, _ := url.Parse("http://127.0.0.1:3001/tcp/" + port)
  proxy := httputil.NewSingleHostReverseProxy(target)

  box := chi.URLParam(r, "box")

  r.URL.Path = strings.TrimPrefix(r.URL.Path, fmt.Sprintf("/%s/port/%s", box, port))

  proxy.ServeHTTP(w, r)
}
