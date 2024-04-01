package command

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/paularlott/knot/api/apiv1"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/middleware"
	"github.com/paularlott/knot/proxy"
	"github.com/paularlott/knot/util"
	"github.com/paularlott/knot/web"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/hostrouter"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	serverCmd.Flags().StringP("listen", "l", "", "The address to listen on (default \"127.0.0.1:3000\").\nOverrides the "+CONFIG_ENV_PREFIX+"_LISTEN environment variable if set.")
	serverCmd.Flags().StringP("nameserver", "n", "", "The nameserver to use for SRV lookups (default use system resolver).\nOverrides the "+CONFIG_ENV_PREFIX+"_NAMESERVER environment variable if set.")
	serverCmd.Flags().StringP("url", "u", "", "The URL to use for the server (default \"http://127.0.0.1:3000\").\nOverrides the "+CONFIG_ENV_PREFIX+"_URL environment variable if set.")
	serverCmd.Flags().BoolP("enable-proxy", "", false, "Enable the proxy server functionality.\nOverrides the "+CONFIG_ENV_PREFIX+"_ENABLE_PROXY environment variable if set.")
	serverCmd.Flags().BoolP("terminal-webgl", "", true, "Enable WebGL terminal renderer.\nOverrides the "+CONFIG_ENV_PREFIX+"_WEBGL environment variable if set.")
	serverCmd.Flags().StringP("download-path", "", "", "The path to serve download files from if set.\nOverrides the "+CONFIG_ENV_PREFIX+"_DOWNLOAD_PATH environment variable if set.")
	serverCmd.Flags().StringP("wildcard-domain", "", "", "The wildcard domain to use for proxying to spaces.\nOverrides the "+CONFIG_ENV_PREFIX+"_WILDCARD_DOMAIN environment variable if set.")
	serverCmd.Flags().StringP("encrypt", "", "", "The encryption key to use for encrypting stored variables.\nOverrides the "+CONFIG_ENV_PREFIX+"_ENCRYPT environment variable if set.")
	serverCmd.Flags().StringP("agent-url", "", "", "The URL agents should use to talk to the server (default \"\").\nOverrides the "+CONFIG_ENV_PREFIX+"_AGENT_URL environment variable if set.")
	serverCmd.Flags().StringP("location", "", "", "The location of the server (defaults to the hostname).\nOverrides the "+CONFIG_ENV_PREFIX+"_LOCATION environment variable if set.")
	serverCmd.Flags().StringP("core-server", "", "", "The address of the core server this server is to become a remote of (default \"\").\nOverrides the "+CONFIG_ENV_PREFIX+"_CORE_SERVER environment variable if set.")
	serverCmd.Flags().StringP("remote-token", "", "", "The token to use for remote and core server communication (default \"\").\nOverrides the "+CONFIG_ENV_PREFIX+"_REMOTE_TOKEN environment variable if set.")

	// TLS
	serverCmd.Flags().StringP("cert-file", "", "", "The file with the PEM encoded certificate to use for the server.\nOverrides the "+CONFIG_ENV_PREFIX+"_CERT_FILE environment variable if set.")
	serverCmd.Flags().StringP("key-file", "", "", "The file with the PEM encoded key to use for the server.\nOverrides the "+CONFIG_ENV_PREFIX+"_KEY_FILE environment variable if set.")
	serverCmd.Flags().BoolP("use-tls", "", true, "Enable TLS.\nOverrides the "+CONFIG_ENV_PREFIX+"_USE_TLS environment variable if set.")
	serverCmd.Flags().BoolP("agent-use-tls", "", true, "Enable TLS when talking to agents.\nOverrides the "+CONFIG_ENV_PREFIX+"_AGENT_USE_TLS environment variable if set.")
	serverCmd.Flags().BoolP("tls-skip-verify", "", true, "Skip TLS verification when talking to agents.\nOverrides the "+CONFIG_ENV_PREFIX+"_TLS_SKIP_VERIFY environment variable if set.")

	// Nomad
	serverCmd.Flags().StringP("nomad-addr", "", "http://127.0.0.1:4646", "The address of the Nomad server (default \"http://127.0.0.1:4646\").\nOverrides the "+CONFIG_ENV_PREFIX+"_NOMAD_ADDR environment variable if set.")
	serverCmd.Flags().StringP("nomad-token", "", "", "The token to use for Nomad API requests.\nOverrides the "+CONFIG_ENV_PREFIX+"_NOMAD_TOKEN environment variable if set.")

	// MySQL
	serverCmd.Flags().BoolP("mysql-enabled", "", false, "Enable MySQL database backend.\nOverrides the "+CONFIG_ENV_PREFIX+"_MYSQL_ENABLED environment variable if set.")
	serverCmd.Flags().StringP("mysql-host", "", "", "The MySQL host to connect to (default \"localhost\").\nOverrides the "+CONFIG_ENV_PREFIX+"_MYSQL_HOST environment variable if set.")
	serverCmd.Flags().IntP("mysql-port", "", 3306, "The MySQL port to connect to (default \"3306\").\nOverrides the "+CONFIG_ENV_PREFIX+"_MYSQL_PORT environment variable if set.")
	serverCmd.Flags().StringP("mysql-user", "", "root", "The MySQL user to connect as (default \"root\").\nOverrides the "+CONFIG_ENV_PREFIX+"_MYSQL_USER environment variable if set.")
	serverCmd.Flags().StringP("mysql-password", "", "", "The MySQL password to use.\nOverrides the "+CONFIG_ENV_PREFIX+"_MYSQL_PASSWORD environment variable if set.")
	serverCmd.Flags().StringP("mysql-database", "", "knot", "The MySQL database to use (default \"knot\").\nOverrides the "+CONFIG_ENV_PREFIX+"_MYSQL_DATABASE environment variable if set.")
	serverCmd.Flags().IntP("mysql-connection-max-idle", "", 2, "The maximum number of idle connections in the connection pool (default \"10\").\nOverrides the "+CONFIG_ENV_PREFIX+"_MYSQL_CONNECTION_MAX_IDLE environment variable if set.")
	serverCmd.Flags().IntP("mysql-connection-max-open", "", 100, "The maximum number of open connections to the database (default \"100\").\nOverrides the "+CONFIG_ENV_PREFIX+"_MYSQL_CONNECTION_MAX_OPEN environment variable if set.")
	serverCmd.Flags().IntP("mysql-connection-max-lifetime", "", 5, "The maximum amount of time in minutes a connection may be reused (default \"5\").\nOverrides the "+CONFIG_ENV_PREFIX+"_MYSQL_CONNECTION_MAX_LIFETIME environment variable if set.")

	// BadgerDB
	serverCmd.Flags().BoolP("badgerdb-enabled", "", false, "Enable BadgerDB database backend.\nOverrides the "+CONFIG_ENV_PREFIX+"_BADGERDB_ENABLED environment variable if set.")
	serverCmd.Flags().StringP("badgerdb-path", "", "./badger", "The path to the BadgerDB database (default \"./badger\").\nOverrides the "+CONFIG_ENV_PREFIX+"_BADGERDB_PATH environment variable if set.")

	// Redis
	serverCmd.Flags().BoolP("redis-enabled", "", false, "Enable Redis database backend.\nOverrides the "+CONFIG_ENV_PREFIX+"_REDIS_ENABLED environment variable if set.")
	serverCmd.Flags().StringP("redis-host", "", "localhost:6379", "The redis server (default \"localhost:6379\").\nOverrides the "+CONFIG_ENV_PREFIX+"_REDIS_HOST environment variable if set.")
	serverCmd.Flags().StringP("redis-password", "", "", "The password to use for the redis server.\nOverrides the "+CONFIG_ENV_PREFIX+"_REDIS_PASSWORD environment variable if set.")
	serverCmd.Flags().IntP("redis-db", "", 0, "The redis database to use (default \"0\").\nOverrides the "+CONFIG_ENV_PREFIX+"_REDIS_DB environment variable if set.")

	RootCmd.AddCommand(serverCmd)
}

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the knot server",
	Long:  `Start the knot server and listen for incoming connections.`,
	Args:  cobra.NoArgs,
	PreRun: func(cmd *cobra.Command, args []string) {
		viper.BindPFlag("server.listen", cmd.Flags().Lookup("listen"))
		viper.BindEnv("server.listen", CONFIG_ENV_PREFIX+"_LISTEN")
		viper.SetDefault("server.listen", "127.0.0.1:3000")

		viper.BindPFlag("server.url", cmd.Flags().Lookup("url"))
		viper.BindEnv("server.url", CONFIG_ENV_PREFIX+"_URL")
		viper.SetDefault("server.url", "http://127.0.0.1:3000")

		viper.BindPFlag("server.wildcard_domain", cmd.Flags().Lookup("wildcard-domain"))
		viper.BindEnv("server.wildcard_domain", CONFIG_ENV_PREFIX+"_WILDCARD_DOMAIN")
		viper.SetDefault("server.wildcard_domain", "")

		viper.BindPFlag("server.nameserver", cmd.Flags().Lookup("nameserver"))
		viper.BindEnv("server.nameserver", CONFIG_ENV_PREFIX+"_NAMESERVER")
		viper.SetDefault("server.nameserver", "")

		viper.BindPFlag("server.enable_proxy", cmd.Flags().Lookup("enable-proxy"))
		viper.BindEnv("server.enable_proxy", CONFIG_ENV_PREFIX+"_ENABLE_PROXY")
		viper.SetDefault("server.enable_proxy", false)

		viper.BindPFlag("server.terminal.webgl", cmd.Flags().Lookup("terminal-webgl"))
		viper.BindEnv("server.terminal.webgl", CONFIG_ENV_PREFIX+"_WEBGL")
		viper.SetDefault("server.terminal.webgl", true)

		viper.BindPFlag("server.download_path", cmd.Flags().Lookup("download-path"))
		viper.BindEnv("server.download_path", CONFIG_ENV_PREFIX+"_DOWNLOAD_PATH")
		viper.SetDefault("server.download_path", "")

		viper.BindPFlag("server.encrypt", cmd.Flags().Lookup("encrypt"))
		viper.BindEnv("server.encrypt", CONFIG_ENV_PREFIX+"_ENCRYPT")
		viper.SetDefault("server.encrypt", "")

		viper.BindPFlag("server.agent_url", cmd.Flags().Lookup("agent-url"))
		viper.BindEnv("server.agent_url", CONFIG_ENV_PREFIX+"_AGENT_URL")
		viper.SetDefault("server.agent_url", "")

		// Get the hostname
		hostname, err := os.Hostname()
		if err != nil {
			log.Fatal().Msgf("Error getting hostname: %v", err)
		}
		hostname = strings.Split(hostname, ".")[0]

		viper.BindPFlag("server.location", cmd.Flags().Lookup("location"))
		viper.BindEnv("server.location", CONFIG_ENV_PREFIX+"_LOCATION")
		viper.SetDefault("server.location", hostname)

		viper.BindPFlag("server.core_server", cmd.Flags().Lookup("core-server"))
		viper.BindEnv("server.core_server", CONFIG_ENV_PREFIX+"_CORE_SERVER")
		viper.SetDefault("server.core_server", "")

		viper.BindPFlag("server.remote_token", cmd.Flags().Lookup("remote-token"))
		viper.BindEnv("server.remote_token", CONFIG_ENV_PREFIX+"_REMOTE_TOKEN")
		viper.SetDefault("server.remote_token", "")

		// TLS
		viper.BindPFlag("server.tls.cert_file", cmd.Flags().Lookup("cert-file"))
		viper.BindEnv("server.tls.cert_file", CONFIG_ENV_PREFIX+"_CERT_FILE")
		viper.SetDefault("server.tls.cert_file", "")

		viper.BindPFlag("server.tls.key_file", cmd.Flags().Lookup("key-file"))
		viper.BindEnv("server.tls.key_file", CONFIG_ENV_PREFIX+"_KEY_FILE")
		viper.SetDefault("server.tls.key_file", "")

		viper.BindPFlag("server.tls.use_tls", cmd.Flags().Lookup("use-tls"))
		viper.BindEnv("server.tls.use_tls", CONFIG_ENV_PREFIX+"_USE_TLS")
		viper.SetDefault("server.tls.use_tls", true)

		viper.BindPFlag("server.tls.agent_use_tls", cmd.Flags().Lookup("agent-use-tls"))
		viper.BindEnv("server.tls.agent_use_tls", CONFIG_ENV_PREFIX+"_AGENT_USE_TLS")
		viper.SetDefault("server.tls.agent_use_tls", true)

		viper.BindPFlag("tls_skip_verify", cmd.Flags().Lookup("tls-skip-verify"))
		viper.BindEnv("tls_skip_verify", CONFIG_ENV_PREFIX+"_TLS_SKIP_VERIFY")
		viper.SetDefault("tls_skip_verify", true)

		// Nomad
		viper.BindPFlag("server.nomad.addr", cmd.Flags().Lookup("nomad-addr"))
		viper.BindEnv("server.nomad.addr", CONFIG_ENV_PREFIX+"_NOMAD_ADDR")
		viper.SetDefault("server.nomad.addr", "http://127.0.0.1:4646")

		viper.BindPFlag("server.nomad.token", cmd.Flags().Lookup("nomad-token"))
		viper.BindEnv("server.nomad.token", CONFIG_ENV_PREFIX+"_NOMAD_TOKEN")
		viper.SetDefault("server.nomad.token", "")

		// MySQL
		viper.BindPFlag("server.mysql.enabled", cmd.Flags().Lookup("mysql-enabled"))
		viper.BindEnv("server.mysql.enabled", CONFIG_ENV_PREFIX+"_MYSQL_ENABLED")
		viper.SetDefault("server.mysql.enabled", false)
		viper.BindPFlag("server.mysql.host", cmd.Flags().Lookup("mysql-host"))
		viper.BindEnv("server.mysql.host", CONFIG_ENV_PREFIX+"_MYSQL_HOST")
		viper.SetDefault("server.mysql.host", "localhost")
		viper.BindPFlag("server.mysql.port", cmd.Flags().Lookup("mysql-port"))
		viper.BindEnv("server.mysql.port", CONFIG_ENV_PREFIX+"_MYSQL_PORT")
		viper.SetDefault("server.mysql.port", 3306)
		viper.BindPFlag("server.mysql.user", cmd.Flags().Lookup("mysql-user"))
		viper.BindEnv("server.mysql.user", CONFIG_ENV_PREFIX+"_MYSQL_USER")
		viper.SetDefault("server.mysql.user", "root")
		viper.BindPFlag("server.mysql.password", cmd.Flags().Lookup("mysql-password"))
		viper.BindEnv("server.mysql.password", CONFIG_ENV_PREFIX+"_MYSQL_PASSWORD")
		viper.SetDefault("server.mysql.password", "")
		viper.BindPFlag("server.mysql.database", cmd.Flags().Lookup("mysql-database"))
		viper.BindEnv("server.mysql.database", CONFIG_ENV_PREFIX+"_MYSQL_DATABASE")
		viper.SetDefault("server.mysql.database", "knot")
		viper.BindPFlag("server.mysql.connection_max_idle", cmd.Flags().Lookup("mysql-connection-max-idle"))
		viper.BindEnv("server.mysql.connection_max_idle", CONFIG_ENV_PREFIX+"_MYSQL_CONNECTION_MAX_IDLE")
		viper.SetDefault("server.mysql.connection_max_idle", 10)
		viper.BindPFlag("server.mysql.connection_max_open", cmd.Flags().Lookup("mysql-connection-max-open"))
		viper.BindEnv("server.mysql.connection_max_open", CONFIG_ENV_PREFIX+"_MYSQL_CONNECTION_MAX_OPEN")
		viper.SetDefault("server.mysql.connection_max_open", 100)
		viper.BindPFlag("server.mysql.connection_max_lifetime", cmd.Flags().Lookup("mysql-connection-max-lifetime"))
		viper.BindEnv("server.mysql.connection_max_lifetime", CONFIG_ENV_PREFIX+"_MYSQL_CONNECTION_MAX_LIFETIME")
		viper.SetDefault("server.mysql.connection_max_lifetime", 5)

		// BadgerDB
		viper.BindPFlag("server.badgerdb.enabled", cmd.Flags().Lookup("badgerdb-enabled"))
		viper.BindEnv("server.badgerdb.enabled", CONFIG_ENV_PREFIX+"_BADGERDB_ENABLED")
		viper.SetDefault("server.badgerdb.enabled", false)
		viper.BindPFlag("server.badgerdb.path", cmd.Flags().Lookup("badgerdb-path"))
		viper.BindEnv("server.badgerdb.path", CONFIG_ENV_PREFIX+"_BADGERDB_PATH")
		viper.SetDefault("server.badgerdb.path", "./badger")

		// Redis
		viper.BindPFlag("server.redis.enabled", cmd.Flags().Lookup("redis-enabled"))
		viper.BindEnv("server.redis.enabled", CONFIG_ENV_PREFIX+"_REDIS_ENABLED")
		viper.SetDefault("server.redis.enabled", false)
		viper.BindPFlag("server.redis.host", cmd.Flags().Lookup("redis-host"))
		viper.BindEnv("server.redis.host", CONFIG_ENV_PREFIX+"_REDIS_HOST")
		viper.SetDefault("server.redis.host", "localhost:6379")
		viper.BindPFlag("server.redis.password", cmd.Flags().Lookup("redis-password"))
		viper.BindEnv("server.redis.password", CONFIG_ENV_PREFIX+"_REDIS_PASSWORD")
		viper.SetDefault("server.redis.password", "")
		viper.BindPFlag("server.redis.db", cmd.Flags().Lookup("redis-db"))
		viper.BindEnv("server.redis.db", CONFIG_ENV_PREFIX+"_REDIS_DB")
		viper.SetDefault("server.redis.db", 0)

		// Set if remote or core server
		viper.Set("server.is_remote", viper.GetString("server.remote_token") != "" && viper.GetString("server.core_server") != "")
		viper.Set("server.is_core", viper.GetString("server.remote_token") != "" && viper.GetString("server.core_server") == "")
	},
	Run: func(cmd *cobra.Command, args []string) {
		listen := viper.GetString("server.listen")

		log.Info().Msgf("server: starting on: %s", listen)

		// Initialize the middleware, test if users are present
		middleware.Initialize()

		// Check manual template is present, create it if not
		if !viper.GetBool("server.is_remote") {
			db := database.GetInstance()
			tpl, err := db.GetTemplate(model.MANUAL_TEMPLATE_ID)
			if err != nil || tpl == nil {
				template := model.NewTemplate("Manual-Configuration", "Access a manually installed agent.", "manual", "", "", []string{})
				template.Id = model.MANUAL_TEMPLATE_ID
				db.SaveTemplate(template)
			}
		} else {
			startRemoteServerServices()
		}

		router := chi.NewRouter()

		// Get the wildcard domain, if blank just start up the server to respond on any domain
		wildcardDomain := viper.GetString("server.wildcard_domain")
		if wildcardDomain == "" {
			router.Mount("/api/v1", apiv1.ApiRoutes())
			router.Mount("/proxy", proxy.Routes())
			router.Mount("/", web.Routes())
			router.Get("/health", web.HandleHealthPage)
		} else {
			// Get the main host domain
			serverURL := viper.GetString("server.url")
			u, err := url.Parse(serverURL)
			if err != nil {
				log.Fatal().Msg(err.Error())
			}

			log.Debug().Msgf("Host: %s", u.Host)
			log.Debug().Msgf("Wildcard Domain: %s", wildcardDomain)
			log.Debug().Msgf("Agent URL: %s", viper.GetString("server.agent_url"))

			hr := hostrouter.New()
			hr.Map(wildcardDomain, func() chi.Router {
				router := chi.NewRouter()
				router.Mount("/", proxy.PortRoutes())
				return router
			}())

			// Expose the health endpoint
			hr.Map("*", func() chi.Router {
				router := chi.NewRouter()
				router.Mount("/api/v1", apiv1.ApiRoutes())
				router.Mount("/proxy", proxy.Routes())
				router.Mount("/", web.Routes())
				router.Get("/health", web.HandleHealthPage)
				return router
			}())

			router.Mount("/", hr)
		}

		var tlsConfig *tls.Config = nil

		// If server should use TLS
		useTLS := viper.GetBool("server.tls.use_tls")
		if useTLS {
			log.Debug().Msg("server: using TLS")

			// If have both a cert and key file, use them
			certFile := viper.GetString("server.tls.cert_file")
			keyFile := viper.GetString("server.tls.key_file")
			if certFile != "" && keyFile != "" {
				log.Info().Msgf("server: using cert file: %s", certFile)
				log.Info().Msgf("server: using key file: %s", keyFile)

				serverTLSCert, err := tls.LoadX509KeyPair(certFile, keyFile)
				if err != nil {
					log.Fatal().Msgf("Error loading certificate and key file: %v", err)
				}

				tlsConfig = &tls.Config{
					Certificates: []tls.Certificate{serverTLSCert},
				}
			} else {
				// Otherwise generate a self-signed cert
				log.Info().Msg("server: generating self-signed certificate")

				// Build the list of domains to include in the cert
				var sslDomains []string

				serverURL := viper.GetString("server.url")
				u, err := url.Parse(serverURL)
				if err != nil {
					log.Fatal().Msg(err.Error())
				}
				sslDomains = append(sslDomains, u.Host)
				sslDomains = append(sslDomains, "localhost")

				// If have an agent url then add it's domain
				agentURL := viper.GetString("server.agent_url")
				if agentURL != "" {
					u, err := url.Parse(agentURL)
					if err != nil {
						log.Fatal().Msg(err.Error())
					}
					sslDomains = append(sslDomains, u.Host)
				}

				// If wildcard domain given add it
				wildcardDomain := viper.GetString("server.wildcard_domain")
				if wildcardDomain != "" {
					sslDomains = append(sslDomains, wildcardDomain)
				}

				cert, key, err := util.GenerateCertificate(sslDomains, []net.IP{net.ParseIP("127.0.0.1")})
				if err != nil {
					log.Fatal().Msgf("Error generating certificate and key: %v", err)
				}

				serverTLSCert, err := tls.X509KeyPair([]byte(cert), []byte(key))
				if err != nil {
					log.Fatal().Msgf("Error generating server TLS cert: %v", err)
				}

				tlsConfig = &tls.Config{
					Certificates: []tls.Certificate{serverTLSCert},
				}
			}
		}

		// Run the http server
		server := &http.Server{
			Addr:         listen,
			Handler:      router,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
			TLSConfig:    tlsConfig,
		}

		if useTLS {
			go func() {
				if err := server.ListenAndServeTLS("", ""); err != http.ErrServerClosed {
					log.Fatal().Msg(err.Error())
				}
			}()
		} else {
			go func() {
				if err := server.ListenAndServe(); err != http.ErrServerClosed {
					log.Fatal().Msg(err.Error())
				}
			}()
		}

		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)

		// Block until we receive our signal.
		<-c

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		server.Shutdown(ctx)
		fmt.Println("\r")
		log.Info().Msg("server: shutdown")
		os.Exit(0)
	},
}

func startRemoteServerServices() {
	log.Info().Msg("server: starting remote server session refresh services")

	// Start a go routine that runs once per hour and pings all sessions to keep them alive
	go func() {
		for {
			time.Sleep(1 * time.Hour)

			log.Debug().Msg("server: refreshing remote sessions")

			db := database.GetCacheInstance()
			sessions, err := db.GetSessions()
			if err != nil {
				log.Error().Msgf("failed to get sessions: %s", err.Error())
				continue
			}

			for _, session := range sessions {
				if session.RemoteSessionId != "" {
					client := apiclient.NewRemoteSession(session.RemoteSessionId)
					_, err := client.Ping()
					if err != nil {
						log.Error().Msgf("failed to ping session: %s", err.Error())
					}
				}
			}
		}
	}()

	// Start a go routine to register with the core server and keep the registration alive
	go func() {
		log.Info().Msg("server: starting remote server registration")

		for {
			client := apiclient.NewRemoteServerClient(viper.GetString("server.core_server"))

			// Register the server with the core server
			serverId, err := client.RegisterRemoteServer(viper.GetString("server.url"))
			if err != nil {
				log.Error().Msgf("failed to register remote server: %s", err.Error())
				time.Sleep(model.REMOTE_SERVER_PING_INTERVAL)
				continue
			} else {
				log.Info().Msgf("server: registered with core server as %s", serverId)
				go syncCachedItems(client)
			}

			for {
				time.Sleep(model.REMOTE_SERVER_PING_INTERVAL)

				// Ping the core server to keep the registration alive
				err = client.UpdateRemoteServer(serverId)
				if err != nil {
					log.Error().Msgf("failed to update remote server: %s", err.Error())
					break
				}
			}
		}
	}()

	// Start a go routine to fetch the list of template hashes from the core server
	go apiv1.SyncTemplateHashes()
}

var syncMutex = &sync.Mutex{}

func syncCachedItems(client *apiclient.ApiClient) {
	syncMutex.Lock()
	defer syncMutex.Unlock()

	db := database.GetInstance()

	log.Info().Msg("server: syncing users from core server")
	users, err := db.GetUsers()
	if err != nil {
		log.Error().Msgf("server: failed to get users: %s", err.Error())
	} else {
		for _, user := range users {
			log.Debug().Msgf("server: syncing user %s", user.Username)
			remoteUser, err := client.RemoteGetUser(user.Id)
			if err != nil {
				// If error is user not found, delete the user
				if strings.Contains(err.Error(), "user not found") {
					log.Debug().Msgf("server: deleting user %s", user.Username)
					apiv1.DeleteUser(db, user)
				} else {
					log.Error().Msgf("server: failed to get user %s: %s", user.Username, err.Error())
				}
			} else {
				db.SaveUser(remoteUser)
				apiv1.UpdateUserSpaces(remoteUser)
			}
		}
	}
	log.Info().Msg("server: finished user sync from core server")
}
