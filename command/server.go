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
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/api"
	"github.com/paularlott/knot/internal/api/api_utils"
	"github.com/paularlott/knot/internal/chat"
	"github.com/paularlott/knot/internal/cluster"
	"github.com/paularlott/knot/internal/config"
	containerHelper "github.com/paularlott/knot/internal/container/helper"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/dns"
	"github.com/paularlott/knot/internal/mcp"
	"github.com/paularlott/knot/internal/middleware"
	"github.com/paularlott/knot/internal/proxy"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/tunnel_server"
	"github.com/paularlott/knot/internal/util"
	"github.com/paularlott/knot/internal/util/audit"
	"github.com/paularlott/knot/web"

	"github.com/paularlott/cli"
	"github.com/rs/zerolog/log"
)

var ServerCmd = &cli.Command{
	Name:        "server",
	Usage:       "Start the knot server",
	Description: "Start the knot server and listen for incoming connections.",
	MaxArgs:     cli.NoArgs,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:         "listen",
			Aliases:      []string{"l"},
			Usage:        "The address to listen on.",
			ConfigPath:   []string{"server.listen"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_LISTEN"},
			DefaultValue: ":3000",
		},
		&cli.StringFlag{
			Name:         "listen-agent",
			Usage:        "The address to listen on for agent connections.",
			ConfigPath:   []string{"server.listen_agent"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_LISTEN_AGENT"},
			DefaultValue: "127.0.0.1:3010",
		},
		&cli.StringFlag{
			Name:         "listen-tunnel",
			Usage:        "The address to listen on for tunnel connections.",
			ConfigPath:   []string{"server.listen_tunnel"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_LISTEN_TUNNEL"},
			DefaultValue: "",
		},
		&cli.StringFlag{
			Name:         "url",
			Aliases:      []string{"u"},
			Usage:        "The URL to use for the server.",
			ConfigPath:   []string{"server.url"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_URL"},
			DefaultValue: "http://127.0.0.1:3000",
		},
		&cli.StringFlag{
			Name:         "tunnel-server",
			Usage:        "The URL for the tunnel client to connect to the individual server.",
			ConfigPath:   []string{"server.tunnel_server"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_TUNNEL_SERVER"},
			DefaultValue: "",
		},
		&cli.BoolFlag{
			Name:         "terminal-webgl",
			Usage:        "Enable WebGL terminal renderer.",
			ConfigPath:   []string{"server.terminal.webgl"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_WEBGL"},
			DefaultValue: true,
		},
		&cli.StringFlag{
			Name:         "download-path",
			Usage:        "The path to serve download files from if set.",
			ConfigPath:   []string{"server.download_path"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_DOWNLOAD_PATH"},
			DefaultValue: "",
		},
		&cli.StringFlag{
			Name:         "wildcard-domain",
			Usage:        "The wildcard domain to use for proxying to spaces.",
			ConfigPath:   []string{"server.wildcard_domain"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_WILDCARD_DOMAIN"},
			DefaultValue: "",
		},
		&cli.StringFlag{
			Name:         "encrypt",
			Usage:        "The encryption key to use for encrypting stored variables.",
			ConfigPath:   []string{"server.encrypt"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_ENCRYPT"},
			DefaultValue: "",
		},
		&cli.StringFlag{
			Name:         "agent-endpoint",
			Usage:        "The address agents should use to talk to the server.",
			ConfigPath:   []string{"server.agent_endpoint"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_AGENT_ENDPOINT"},
			DefaultValue: "",
		},
		&cli.StringFlag{
			Name:       "zone",
			Usage:      "The zone of the server.",
			ConfigPath: []string{"server.zone"},
			EnvVars:    []string{config.CONFIG_ENV_PREFIX + "_ZONE"},
		},
		&cli.StringFlag{
			Name:         "html-path",
			Usage:        "The optional path to the html files to serve.",
			ConfigPath:   []string{"server.html_path"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_HTML_PATH"},
			DefaultValue: "",
		},
		&cli.StringFlag{
			Name:         "template-path",
			Usage:        "The optional path to the template files to serve.",
			ConfigPath:   []string{"server.template_path"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_TEMPLATE_PATH"},
			DefaultValue: "",
		},
		&cli.StringFlag{
			Name:         "agent-path",
			Usage:        "The optional path to the agent files to serve.",
			ConfigPath:   []string{"server.agent_path"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_AGENT_PATH"},
			DefaultValue: "",
		},
		&cli.StringFlag{
			Name:         "timezone",
			Usage:        "The timezone to use for the server.",
			ConfigPath:   []string{"server.timezone"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_TIMEZONE"},
			DefaultValue: "",
		},
		&cli.StringFlag{
			Name:         "tunnel-domain",
			Usage:        "The domain to use for tunnel connections.",
			ConfigPath:   []string{"server.tunnel_domain"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_TUNNEL_DOMAIN"},
			DefaultValue: "",
		},
		&cli.IntFlag{
			Name:         "audit-retention",
			Usage:        "The number of days to keep audit logs.",
			ConfigPath:   []string{"server.audit_retention"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_AUDIT_RETENTION"},
			DefaultValue: 90,
		},
		&cli.BoolFlag{
			Name:         "disable-space-create",
			Usage:        "Disable the ability to create spaces.",
			ConfigPath:   []string{"server.disable_space_create"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_DISABLE_SPACE_CREATE"},
			DefaultValue: false,
		},
		&cli.BoolFlag{
			Name:         "auth-ip-rate-limiting",
			Usage:        "Enable IP rate limiting of authentication.",
			ConfigPath:   []string{"server.auth_ip_rate_limiting"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_AUTH_IP_RATE_LIMITING"},
			DefaultValue: true,
		},
		&cli.StringFlag{
			Name:         "public-files-path",
			Usage:        "The path to the a directory to serve as /public-files.",
			ConfigPath:   []string{"server.public_files_path"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_PUBLIC_FILES_PATH"},
			DefaultValue: "",
		},
		&cli.StringFlag{
			Name:         "private-files-path",
			Usage:        "The path to the a directory to serve as /private-files.",
			ConfigPath:   []string{"server.private_files_path"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_PRIVATE_FILES_PATH"},
			DefaultValue: "",
		},

		// UI flags
		&cli.BoolFlag{
			Name:         "hide-support-links",
			Usage:        "Hide the support links in the UI.",
			ConfigPath:   []string{"server.ui.hide_support_links"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_HIDE_SUPPORT_LINKS"},
			DefaultValue: false,
		},
		&cli.BoolFlag{
			Name:         "hide-api-tokens",
			Usage:        "Hide the API tokens menu item in the UI.",
			ConfigPath:   []string{"server.ui.hide_api_tokens"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_HIDE_API_TOKENS"},
			DefaultValue: false,
		},
		&cli.BoolFlag{
			Name:         "enable-gravatar",
			Usage:        "Enable Gravatar support in the UI.",
			ConfigPath:   []string{"server.ui.enable_gravatar"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_ENABLE_GRAVATAR"},
			DefaultValue: true,
		},
		&cli.StringSliceFlag{
			Name:       "icons",
			Usage:      "File defining icons for use with templates and spaces, can be given multiple times.",
			ConfigPath: []string{"server.ui.icons"},
			EnvVars:    []string{config.CONFIG_ENV_PREFIX + "_ICONS"},
		},
		&cli.BoolFlag{
			Name:         "enable-builtin-icons",
			Usage:        "Enable the use of the built-in icons.",
			ConfigPath:   []string{"server.ui.enable_builtin_icons"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_ENABLE_BUILTIN_ICONS"},
			DefaultValue: true,
		},
		&cli.StringFlag{
			Name:         "logo-url",
			Usage:        "The URL to the logo to use in the UI.",
			ConfigPath:   []string{"server.ui.logo_url"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_LOGO_URL"},
			DefaultValue: "",
		},
		&cli.BoolFlag{
			Name:         "logo-invert",
			Usage:        "Invert the logo colors in the UI for dark mode.",
			ConfigPath:   []string{"server.ui.logo_invert"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_LOGO_INVERT"},
			DefaultValue: false,
		},

		// Cluster flags
		&cli.StringFlag{
			Name:         "cluster-key",
			Usage:        "The shared cluster key.",
			ConfigPath:   []string{"server.cluster.key"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_CLUSTER_KEY"},
			DefaultValue: "",
		},
		&cli.StringFlag{
			Name:         "cluster-advertise-addr",
			Usage:        "The address to advertise to other servers.",
			ConfigPath:   []string{"server.cluster.advertise_addr"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_CLUSTER_ADVERTISE_ADDR"},
			DefaultValue: "",
		},
		&cli.StringFlag{
			Name:         "cluster-bind-addr",
			Usage:        "The address to bind to for cluster communication.",
			ConfigPath:   []string{"server.cluster.bind_addr"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_CLUSTER_BIND_ADDR"},
			DefaultValue: "",
		},
		&cli.StringSliceFlag{
			Name:       "cluster-peer",
			Usage:      "The addresses of the other servers in the cluster, can be given multiple times.",
			ConfigPath: []string{"server.cluster.peers"},
			EnvVars:    []string{config.CONFIG_ENV_PREFIX + "_CLUSTER_PEERS"},
		},
		&cli.BoolFlag{
			Name:         "allow-leaf-nodes",
			Usage:        "Allow leaf nodes to connect to the cluster.",
			ConfigPath:   []string{"server.cluster.allow_leaf_nodes"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_ALLOW_LEAF_NODES"},
			DefaultValue: true,
		},
		&cli.BoolFlag{
			Name:         "cluster-compression",
			Usage:        "Enable compression for cluster communication.",
			ConfigPath:   []string{"server.cluster.compression"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_CLUSTER_COMPRESSION"},
			DefaultValue: true,
		},

		// Origin / Leaf server flags
		&cli.StringFlag{
			Name:         "origin-server",
			Usage:        "The address of the origin server.",
			ConfigPath:   []string{"server.origin.server"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_ORIGIN_SERVER"},
			DefaultValue: "",
		},
		&cli.StringFlag{
			Name:         "origin-token",
			Usage:        "The token to use for the origin server.",
			ConfigPath:   []string{"server.origin.token"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_ORIGIN_TOKEN"},
			DefaultValue: "",
		},

		// TOTP flags
		&cli.BoolFlag{
			Name:         "enable-totp",
			Usage:        "Enable TOTP for users.",
			ConfigPath:   []string{"server.totp.enabled"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_ENABLE_TOTP"},
			DefaultValue: false,
		},
		&cli.IntFlag{
			Name:         "totp-window",
			Usage:        "The number of time steps (30 seconds) to check for TOTP codes.",
			ConfigPath:   []string{"server.totp.window"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_TOTP_WINDOW"},
			DefaultValue: 1,
		},
		&cli.StringFlag{
			Name:         "totp-issuer",
			Usage:        "The issuer to use for TOTP codes.",
			ConfigPath:   []string{"server.totp.issuer"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_TOTP_ISSUER"},
			DefaultValue: "Knot",
		},

		// TLS flags
		&cli.StringFlag{
			Name:         "cert-file",
			Usage:        "The file with the PEM encoded certificate to use for the server.",
			ConfigPath:   []string{"server.tls.cert_file"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_CERT_FILE"},
			DefaultValue: "",
		},
		&cli.StringFlag{
			Name:         "key-file",
			Usage:        "The file with the PEM encoded key to use for the server.",
			ConfigPath:   []string{"server.tls.key_file"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_KEY_FILE"},
			DefaultValue: "",
		},
		&cli.BoolFlag{
			Name:         "use-tls",
			Usage:        "Enable TLS.",
			ConfigPath:   []string{"server.tls.use_tls"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_USE_TLS"},
			DefaultValue: true,
		},
		&cli.BoolFlag{
			Name:         "agent-use-tls",
			Usage:        "Enable TLS when talking to agents.",
			ConfigPath:   []string{"server.tls.agent_use_tls"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_AGENT_USE_TLS"},
			DefaultValue: true,
		},
		&cli.BoolFlag{
			Name:         "tls-skip-verify",
			Usage:        "Skip TLS verification when talking to agents.",
			ConfigPath:   []string{"tls.skip_verify"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_TLS_SKIP_VERIFY"},
			DefaultValue: true,
		},

		// Nomad flags
		&cli.StringFlag{
			Name:         "nomad-addr",
			Usage:        "The address of the Nomad server.",
			ConfigPath:   []string{"server.nomad.addr"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_NOMAD_ADDR"},
			DefaultValue: "http://127.0.0.1:4646",
		},
		&cli.StringFlag{
			Name:         "nomad-token",
			Usage:        "The token to use for Nomad API requests.",
			ConfigPath:   []string{"server.nomad.token"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_NOMAD_TOKEN"},
			DefaultValue: "",
		},

		// MySQL flags
		&cli.BoolFlag{
			Name:         "mysql-enabled",
			Usage:        "Enable MySQL database backend.",
			ConfigPath:   []string{"server.mysql.enabled"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_MYSQL_ENABLED"},
			DefaultValue: false,
		},
		&cli.StringFlag{
			Name:         "mysql-host",
			Usage:        "The MySQL host to connect to.",
			ConfigPath:   []string{"server.mysql.host"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_MYSQL_HOST"},
			DefaultValue: "localhost",
		},
		&cli.IntFlag{
			Name:         "mysql-port",
			Usage:        "The MySQL port to connect to.",
			ConfigPath:   []string{"server.mysql.port"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_MYSQL_PORT"},
			DefaultValue: 3306,
		},
		&cli.StringFlag{
			Name:         "mysql-user",
			Usage:        "The MySQL user to connect as.",
			ConfigPath:   []string{"server.mysql.user"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_MYSQL_USER"},
			DefaultValue: "root",
		},
		&cli.StringFlag{
			Name:         "mysql-password",
			Usage:        "The MySQL password to use.",
			ConfigPath:   []string{"server.mysql.password"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_MYSQL_PASSWORD"},
			DefaultValue: "",
		},
		&cli.StringFlag{
			Name:         "mysql-database",
			Usage:        "The MySQL database to use.",
			ConfigPath:   []string{"server.mysql.database"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_MYSQL_DATABASE"},
			DefaultValue: "knot",
		},
		&cli.IntFlag{
			Name:         "mysql-connection-max-idle",
			Usage:        "The maximum number of idle connections in the connection pool.",
			ConfigPath:   []string{"server.mysql.connection_max_idle"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_MYSQL_CONNECTION_MAX_IDLE"},
			DefaultValue: 10,
		},
		&cli.IntFlag{
			Name:         "mysql-connection-max-open",
			Usage:        "The maximum number of open connections to the database.",
			ConfigPath:   []string{"server.mysql.connection_max_open"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_MYSQL_CONNECTION_MAX_OPEN"},
			DefaultValue: 100,
		},
		&cli.IntFlag{
			Name:         "mysql-connection-max-lifetime",
			Usage:        "The maximum amount of time in minutes a connection may be reused.",
			ConfigPath:   []string{"server.mysql.connection_max_lifetime"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_MYSQL_CONNECTION_MAX_LIFETIME"},
			DefaultValue: 5,
		},

		// BadgerDB flags
		&cli.BoolFlag{
			Name:         "badgerdb-enabled",
			Usage:        "Enable BadgerDB database backend.",
			ConfigPath:   []string{"server.badgerdb.enabled"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_BADGERDB_ENABLED"},
			DefaultValue: false,
		},
		&cli.StringFlag{
			Name:         "badgerdb-path",
			Usage:        "The path to the BadgerDB database.",
			ConfigPath:   []string{"server.badgerdb.path"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_BADGERDB_PATH"},
			DefaultValue: "./badger",
		},

		// Redis flags
		&cli.BoolFlag{
			Name:         "redis-enabled",
			Usage:        "Enable Redis database backend.",
			ConfigPath:   []string{"server.redis.enabled"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_REDIS_ENABLED"},
			DefaultValue: false,
		},
		&cli.StringSliceFlag{
			Name:         "redis-hosts",
			Usage:        "The redis server(s), can be specified multiple times.",
			ConfigPath:   []string{"server.redis.hosts"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_REDIS_HOSTS"},
			DefaultValue: []string{"localhost:6379"},
		},
		&cli.StringFlag{
			Name:         "redis-password",
			Usage:        "The password to use for the redis server.",
			ConfigPath:   []string{"server.redis.password"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_REDIS_PASSWORD"},
			DefaultValue: "",
		},
		&cli.IntFlag{
			Name:         "redis-db",
			Usage:        "The redis database to use.",
			ConfigPath:   []string{"server.redis.db"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_REDIS_DB"},
			DefaultValue: 0,
		},
		&cli.StringFlag{
			Name:         "redis-master-name",
			Usage:        "The name of the master to use for failover clients.",
			ConfigPath:   []string{"server.redis.master_name"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_REDIS_MASTER_NAME"},
			DefaultValue: "",
		},
		&cli.StringFlag{
			Name:         "redis-key-prefix",
			Usage:        "The prefix to use for all keys in the redis database.",
			ConfigPath:   []string{"server.redis.key_prefix"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_REDIS_KEY_PREFIX"},
			DefaultValue: "",
		},

		// Docker flags
		&cli.StringFlag{
			Name:         "docker-host",
			Usage:        "The Docker host to connect to.",
			ConfigPath:   []string{"server.docker.host"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_DOCKER_HOST"},
			DefaultValue: "unix:///var/run/docker.sock",
		},

		// Podman flags
		&cli.StringFlag{
			Name:         "podman-host",
			Usage:        "The Podman host to connect to.",
			ConfigPath:   []string{"server.podman.host"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_PODMAN_HOST"},
			DefaultValue: "unix:///var/run/podman.sock",
		},

		// Chat flags
		&cli.BoolFlag{
			Name:         "chat-enabled",
			Usage:        "Enable AI chat functionality.",
			ConfigPath:   []string{"server.chat.enabled"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_CHAT_ENABLED"},
			DefaultValue: false,
		},
		&cli.StringFlag{
			Name:         "chat-openai-api-key",
			Usage:        "OpenAI API key for chat functionality.",
			ConfigPath:   []string{"server.chat.openai_api_key"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_CHAT_OPENAI_API_KEY"},
			DefaultValue: "",
		},
		&cli.StringFlag{
			Name:         "chat-openai-base-url",
			Usage:        "OpenAI API base URL for chat functionality.",
			ConfigPath:   []string{"server.chat.openai_base_url"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_CHAT_OPENAI_BASE_URL"},
			DefaultValue: "http://127.0.0.1:11434/v1",
		},
		&cli.StringFlag{
			Name:         "chat-model",
			Usage:        "OpenAI model to use for chat.",
			ConfigPath:   []string{"server.chat.model"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_CHAT_MODEL"},
			DefaultValue: "qwen2.5-coder:14b",
		},
		&cli.IntFlag{
			Name:         "chat-max-tokens",
			Usage:        "Maximum tokens for chat responses.",
			ConfigPath:   []string{"server.chat.max_tokens"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_CHAT_MAX_TOKENS"},
			DefaultValue: 8192,
		},
		&cli.Float32Flag{
			Name:         "chat-temperature",
			Usage:        "Temperature for chat responses.",
			ConfigPath:   []string{"server.chat.temperature"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_CHAT_TEMPERATURE"},
			DefaultValue: 0.1,
		},
		&cli.StringFlag{
			Name:       "chat-system-prompt",
			Usage:      "System prompt for chat functionality.",
			ConfigPath: []string{"server.chat.system_prompt"},
			EnvVars:    []string{config.CONFIG_ENV_PREFIX + "_CHAT_SYSTEM_PROMPT"},
			DefaultValue: `You are a helpful assistant for the cloud-based development environment, knot.

You can help users manage their development spaces, start and stop space, and provide information about the system.

You have access to tools that can:
- List spaces and their details
- Start and stop spaces
- Get Docker/Podman specifications
- Provide system information

Guidelines for interactions:
- When users ask about their spaces or want to perform actions, call the appropriate tools to help them
- If a user asks you to create a Docker or Podman job, first call get_docker_podman_spec to get the latest specification, then use it to create the job specification
- If a user asks you to interact with a space by name and you don't know the ID of the space, first call list_spaces to get the list of spaces including their names and IDs, then use the ID you find to interact with the space
- If you can't find the ID of a space, tell the user that you don't know that space - don't guess
- Always use the tools available to you rather than making assumptions about system state
- Provide clear, helpful responses based on the actual results from tool calls
- Do not show tool call JSON in your responses - just use the tools and provide helpful responses based on the results
- You must accept the output from tools as being correct and accurate

Be concise, accurate, and helpful in all interactions.`,
		},

		// DNS flags
		&cli.BoolFlag{
			Name:         "dns-enabled",
			Usage:        "Enable DNS server.",
			ConfigPath:   []string{"server.dns.enabled"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_DNS_ENABLED"},
			DefaultValue: false,
		},
		&cli.StringFlag{
			Name:         "dns-listen",
			Usage:        "The address and port to listen on for DNS queries.",
			ConfigPath:   []string{"server.dns.listen"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_DNS_LISTEN"},
			DefaultValue: ":3053",
		},
		&cli.StringSliceFlag{
			Name:       "dns-records",
			Usage:      "The DNS records to add, can be specified multiple times.",
			ConfigPath: []string{"server.dns.records"},
			EnvVars:    []string{config.CONFIG_ENV_PREFIX + "_DNS_RECORDS"},
		},
		&cli.IntFlag{
			Name:         "dns-default-ttl",
			Usage:        "Default TTL for records if a TTL isn't explicitly set.",
			ConfigPath:   []string{"server.dns.default_ttl"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_DNS_DEFAULT_TTL"},
			DefaultValue: 300,
		},
		&cli.BoolFlag{
			Name:         "dns-enable-upstream",
			Usage:        "Enable resolution of unknown domains by passing to upstream DNS servers.",
			ConfigPath:   []string{"server.dns.enable_upstream"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_DNS_ENABLE_UPSTREAM"},
			DefaultValue: false,
		},
	},
	Run: func(ctx context.Context, cmd *cli.Command) error {
		cfg := buildServerConfig(cmd)

		listen := util.FixListenAddress(cfg.Listen)

		// If agent address not given then don't start
		if cfg.AgentEndpoint == "" {
			log.Fatal().Msg("server: agent endpoint not given")
		}

		log.Info().Msgf("server: starting knot version: %s", build.Version)
		log.Info().Msgf("server: starting on: %s", listen)

		// Initialize the API helpers
		service.SetUserService(api_utils.NewApiUtilsUsers())
		service.SetContainerService(containerHelper.NewContainerHelper())

		// Initialize the middleware, test if users are present
		middleware.Initialize()

		// Load roles into memory cache
		roles, err := database.GetInstance().GetRoles()
		if err != nil {
			log.Fatal().Msgf("server: failed to get roles: %s", err.Error())
		}
		model.SetRoleCache(roles)

		// Start the DNS server if enabled
		if cmd.GetBool("dns-enabled") {
			dnsServerCfg := dns.DNSServerConfig{
				ListenAddr: cmd.GetString("dns-listen"),
				Records:    cmd.GetStringSlice("dns-records"),
				DefaultTTL: cmd.GetInt("dns-default-ttl"),
			}

			if cmd.GetBool("dns-enable-upstream") {
				dnsServerCfg.Resolver = dns.GetDefaultResolver()

				// Enable the resolver cache
				dnsServerCfg.Resolver.SetConfig(dns.ResolverConfig{
					QueryTimeout: 2 * time.Second,
					EnableCache:  true,
					MaxCacheTTL:  30,
				})
			}

			dnsServer, err := dns.NewDNSServer(dnsServerCfg)
			if err != nil {
				log.Fatal().Err(err).Msg("Failed to create DNS server")
			}

			if err = dnsServer.Start(); err != nil {
				log.Fatal().Err(err).Msg("Failed to start DNS server")
			}
			defer dnsServer.Stop()
		}

		var router http.Handler

		// Get the main host domain & wildcard domain
		wildcardDomain := cfg.WildcardDomain
		serverURL := cfg.URL
		u, err := url.Parse(serverURL)
		if err != nil {
			log.Fatal().Msg(err.Error())
		}

		log.Debug().Msgf("Host: %s", u.Host)

		var tunnelServerUrl *url.URL = nil
		if cfg.TunnelServer != "" && cfg.ListenTunnel != "" {
			tunnelServerUrl, err = url.Parse(cfg.TunnelServer)
			if err != nil {
				log.Fatal().Msgf("Error parsing tunnel server URL: %v", err)
			}
			log.Debug().Msgf("Tunnel Server URL: %s", tunnelServerUrl.Host)
		}

		// Create the application routes
		routes := http.NewServeMux()

		api.ApiRoutes(routes)
		proxy.Routes(routes, cfg)
		web.Routes(routes, cfg)

		// MCP
		mcpServer := mcp.InitializeMCPServer(routes)

		// If AI chat enabled then initialize chat service
		if cmd.GetBool("chat-enabled") {
			// Initialize chat service with config
			_, err := chat.NewService(cfg.Chat, mcpServer, routes)
			if err != nil {
				log.Fatal().Msgf("server: failed to create chat service: %s", err.Error())
			}
		}

		// Add support for page not found
		appRoutes := web.HandlePageNotFound(routes)

		if cfg.ListenTunnel != "" {
			tunnel_server.Routes(routes)
		}

		// If have a wildcard domain, build it's routes
		if wildcardDomain != "" {
			log.Debug().Msgf("Wildcard Domain: %s", wildcardDomain)

			// Remove the port form the wildcard domain
			if host, _, err := net.SplitHostPort(wildcardDomain); err == nil {
				wildcardDomain = host
			}

			// Create a regex to match the wildcard domain
			match := regexp.MustCompile("^[a-zA-Z0-9-]+" + strings.TrimLeft(strings.Replace(wildcardDomain, ".", "\\.", -1), "*") + "$")

			// Get our hostname without port if present
			hostname := u.Host
			if host, _, err := net.SplitHostPort(hostname); err == nil {
				hostname = host
			}

			// Get the routes for the wildcard domain
			wildcardRoutes := proxy.PortRoutes()
			domainMux := http.NewServeMux()
			domainMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				// Extract hostname without port if present
				requestHost := r.Host
				if host, _, err := net.SplitHostPort(requestHost); err == nil {
					requestHost = host
				}

				if requestHost == hostname || (tunnelServerUrl != nil && requestHost == tunnelServerUrl.Host) {
					appRoutes.ServeHTTP(w, r)
				} else if match.MatchString(requestHost) {
					wildcardRoutes.ServeHTTP(w, r)
				} else {
					if r.URL.Path == "/health" {
						web.HandleHealthPage(w, r)
					} else {
						http.NotFound(w, r)
					}
				}
			})

			router = domainMux
		} else {
			// No wildcard domain, just use the app routes
			router = appRoutes
		}

		var tlsConfig *tls.Config = nil

		// If server should use TLS
		if cfg.TLS.UseTLS {
			log.Debug().Msg("server: using TLS")

			// If have both a cert and key file, use them
			certFile := cfg.TLS.CertFile
			keyFile := cfg.TLS.KeyFile
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

				serverURL := cfg.URL
				u, err := url.Parse(serverURL)
				if err != nil {
					log.Fatal().Msg(err.Error())
				}
				hostname := u.Host
				if host, _, err := net.SplitHostPort(hostname); err == nil {
					hostname = host
				}

				sslDomains = append(sslDomains, hostname)
				if hostname != "localhost" {
					sslDomains = append(sslDomains, "localhost")
				}

				if tunnelServerUrl != nil {
					sslDomains = append(sslDomains, tunnelServerUrl.Host)
				}

				// If wildcard domain given add it
				wildcardDomain := cfg.WildcardDomain
				if wildcardDomain != "" {
					if host, _, err := net.SplitHostPort(wildcardDomain); err == nil {
						wildcardDomain = host
					}

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

		// Start the gossip server
		cluster := cluster.NewCluster(
			cfg.Cluster.Key,
			cfg.Cluster.AdvertiseAddr,
			cfg.Cluster.BindAddr,
			routes,
			cfg.Cluster.Compression,
			cfg.Cluster.AllowLeafNodes,
		)
		service.SetTransport(cluster)

		// Run the http server
		server := &http.Server{
			Addr:         listen,
			Handler:      router,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 2 * time.Minute, // Extended to support AI time
			TLSConfig:    tlsConfig,
		}

		go func() {
			for {
				if cfg.TLS.UseTLS {
					if err := server.ListenAndServeTLS("", ""); err != http.ErrServerClosed {
						log.Error().Err(err).Msgf("server: web server")
					}
				} else {
					if err := server.ListenAndServe(); err != http.ErrServerClosed {
						log.Error().Err(err).Msgf("server: web server")
					}
				}
			}
		}()

		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)

		// Start the cluster and join the peers
		cluster.Start(
			cfg.Cluster.Peers,
			cfg.Origin.Server,
			cfg.Origin.Token,
		)

		// Check for local spaces that are pending state changes and setup watches
		service.GetContainerService().CleanupOnBoot()

		// Start the agent server
		agent_server.ListenAndServe(util.FixListenAddress(cfg.ListenAgent), tlsConfig)

		// Start a tunnel server
		if cfg.ListenTunnel != "" {
			tunnel_server.ListenAndServe(util.FixListenAddress(cfg.ListenTunnel), tlsConfig)
		}

		audit.Log(
			model.AuditActorSystem,
			model.AuditActorTypeSystem,
			model.AuditEventSystemStart,
			"",
			&map[string]interface{}{
				"build": build.Version,
			},
		)

		// Block until we receive our signal.
		<-c

		// Shutdown the server cluster
		cluster.Stop()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		server.Shutdown(ctx)
		fmt.Print("\r")
		log.Info().Msg("server: shutdown")
		return nil
	},
}

func buildServerConfig(cmd *cli.Command) *config.ServerConfig {
	// Get the hostname with fallback logic
	hostname := os.Getenv("NOMAD_DC")
	if hostname == "" {
		var err error
		hostname, err = os.Hostname()
		if err != nil {
			log.Fatal().Msgf("Error getting hostname: %v", err)
		}
		hostname = strings.Split(hostname, ".")[0]
	}

	// Use hostname as default for zone if not set
	zone := cmd.GetString("zone")
	if zone == "" {
		zone = hostname
	}

	serverCfg := &config.ServerConfig{
		Listen:             cmd.GetString("listen"),
		ListenAgent:        cmd.GetString("listen-agent"),
		URL:                cmd.GetString("url"),
		AgentEndpoint:      cmd.GetString("agent-endpoint"),
		WildcardDomain:     cmd.GetString("wildcard-domain"),
		HTMLPath:           cmd.GetString("html-path"),
		TemplatePath:       cmd.GetString("template-path"),
		AgentPath:          cmd.GetString("agent-path"),
		PrivateFilesPath:   cmd.GetString("private-files-path"),
		PublicFilesPath:    cmd.GetString("public-files-path"),
		DownloadPath:       cmd.GetString("download-path"),
		DisableSpaceCreate: cmd.GetBool("disable-space-create"),
		ListenTunnel:       cmd.GetString("listen-tunnel"),
		TunnelDomain:       cmd.GetString("tunnel-domain"),
		TunnelServer:       cmd.GetString("tunnel-server"),
		TerminalWebGL:      cmd.GetBool("terminal-webgl"),
		EncryptionKey:      cmd.GetString("encrypt"),
		Zone:               zone,
		Timezone:           cmd.GetString("timezone"),
		LeafNode:           cmd.GetString("origin-server") != "" && cmd.GetString("origin-token") != "",
		AuthIPRateLimiting: cmd.GetBool("auth-ip-rate-limiting"),
		Origin: config.OriginConfig{
			Server: cmd.GetString("origin-server"),
			Token:  cmd.GetString("origin-token"),
		},
		TOTP: config.TOTPConfig{
			Enabled: cmd.GetBool("enable-totp"),
			Window:  cmd.GetInt("totp-window"),
			Issuer:  cmd.GetString("totp-issuer"),
		},
		UI: config.UIConfig{
			HideSupportLinks:   cmd.GetBool("hide-support-links"),
			HideAPITokens:      cmd.GetBool("hide-api-tokens"),
			EnableGravatar:     cmd.GetBool("enable-gravatar"),
			LogoURL:            cmd.GetString("logo-url"),
			LogoInvert:         cmd.GetBool("logo-invert"),
			EnableBuiltinIcons: cmd.GetBool("enable-builtin-icons"),
			Icons:              cmd.GetStringSlice("icons"),
		},
		Cluster: config.ClusterConfig{
			Key:            cmd.GetString("cluster-key"),
			AdvertiseAddr:  cmd.GetString("cluster-advertise-addr"),
			BindAddr:       cmd.GetString("cluster-bind-addr"),
			Peers:          cmd.GetStringSlice("cluster-peer"),
			AllowLeafNodes: cmd.GetBool("allow-leaf-nodes"),
			Compression:    cmd.GetBool("cluster-compression"),
		},
		MySQL: config.MySQLConfig{
			Enabled:               cmd.GetBool("mysql-enabled"),
			Host:                  cmd.GetString("mysql-host"),
			Port:                  cmd.GetInt("mysql-port"),
			User:                  cmd.GetString("mysql-user"),
			Password:              cmd.GetString("mysql-password"),
			Database:              cmd.GetString("mysql-database"),
			ConnectionMaxIdle:     cmd.GetInt("mysql-connection-max-idle"),
			ConnectionMaxOpen:     cmd.GetInt("mysql-connection-max-open"),
			ConnectionMaxLifetime: cmd.GetInt("mysql-connection-max-lifetime"),
		},
		BadgerDB: config.BadgerDBConfig{
			Enabled: cmd.GetBool("badgerdb-enabled"),
			Path:    cmd.GetString("badgerdb-path"),
		},
		Redis: config.RedisConfig{
			Enabled:    cmd.GetBool("redis-enabled"),
			Hosts:      cmd.GetStringSlice("redis-hosts"),
			Password:   cmd.GetString("redis-password"),
			DB:         cmd.GetInt("redis-db"),
			MasterName: cmd.GetString("redis-master-name"),
			KeyPrefix:  cmd.GetString("redis-key-prefix"),
		},
		Audit: config.AuditConfig{
			Retention: cmd.GetInt("audit-retention"),
		},
		Docker: config.DockerConfig{
			Host: cmd.GetString("docker-host"),
		},
		Podman: config.PodmanConfig{
			Host: cmd.GetString("podman-host"),
		},
		Nomad: config.NomadConfig{
			Host:  cmd.GetString("nomad-addr"),
			Token: cmd.GetString("nomad-token"),
		},
		TLS: config.TLSConfig{
			CertFile:    cmd.GetString("cert-file"),
			KeyFile:     cmd.GetString("key-file"),
			UseTLS:      cmd.GetBool("use-tls"),
			AgentUseTLS: cmd.GetBool("agent-use-tls"),
			SkipVerify:  cmd.GetBool("tls-skip-verify"),
		},
		Chat: config.ChatConfig{
			Enabled:       cmd.GetBool("chat-enabled"),
			OpenAIAPIKey:  cmd.GetString("chat-openai-api-key"),
			OpenAIBaseURL: cmd.GetString("chat-openai-base-url"),
			Model:         cmd.GetString("chat-model"),
			MaxTokens:     cmd.GetInt("chat-max-tokens"),
			Temperature:   cmd.GetFloat32("chat-temperature"),
			SystemPrompt:  cmd.GetString("chat-system-prompt"),
		},
	}

	// If tunnel domain doesn't start with a . then prefix it, strip leading * if present
	if serverCfg.TunnelDomain != "" {
		serverCfg.TunnelDomain = strings.TrimPrefix(serverCfg.TunnelDomain, "*")

		if !strings.HasPrefix(serverCfg.TunnelDomain, ".") {
			serverCfg.TunnelDomain = "." + serverCfg.TunnelDomain
		}
	}

	// If tunnel server not given then use the instances url as the tunnel server
	if serverCfg.TunnelServer == "" {
		serverCfg.TunnelServer = serverCfg.URL
	}

	// Force the zone for leaf nodes
	if serverCfg.LeafNode {
		serverCfg.Zone = model.LeafNodeZone
	}

	if serverCfg.Timezone == "" {
		serverCfg.Timezone, _ = time.Now().Zone()
	}
	log.Info().Msgf("server: timezone: %s", serverCfg.Timezone)

	config.SetServerConfig(serverCfg)

	return serverCfg
}
