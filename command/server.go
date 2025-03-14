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

	"github.com/paularlott/knot/api"
	"github.com/paularlott/knot/api/api_utils"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/container/nomad"
	"github.com/paularlott/knot/internal/dnsserver"
	"github.com/paularlott/knot/internal/origin_leaf"
	"github.com/paularlott/knot/internal/origin_leaf/server_info"
	"github.com/paularlott/knot/internal/tunnel_server"
	"github.com/paularlott/knot/middleware"
	"github.com/paularlott/knot/proxy"
	"github.com/paularlott/knot/util"
	"github.com/paularlott/knot/util/audit"
	"github.com/paularlott/knot/web"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	serverCmd.Flags().StringP("listen", "l", "", "The address to listen on (default \"127.0.0.1:3000\").\nOverrides the "+config.CONFIG_ENV_PREFIX+"_LISTEN environment variable if set.")
	serverCmd.Flags().StringP("listen-agent", "", "", "The address to listen on for agent connections (default \":3010\").\nOverrides the "+config.CONFIG_ENV_PREFIX+"_LISTEN_AGENT environment variable if set.")
	serverCmd.Flags().StringP("listen-tunnel", "", "", "The address to listen on for tunnel connections (default \"\" disabled).\nOverrides the "+config.CONFIG_ENV_PREFIX+"_LISTEN_TUNNEL environment variable if set.")
	serverCmd.Flags().StringSliceP("nameserver", "", []string{}, "The address of the nameserver to use for SRV lookups, can be given multiple times (default use system resolver).\nOverrides the "+config.CONFIG_ENV_PREFIX+"_NAMESERVERS environment variable if set.")
	serverCmd.Flags().StringP("url", "u", "", "The URL to use for the server (default \"http://127.0.0.1:3000\").\nOverrides the "+config.CONFIG_ENV_PREFIX+"_URL environment variable if set.")
	serverCmd.Flags().BoolP("enable-proxy", "", false, "Enable the proxy server functionality.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_ENABLE_PROXY environment variable if set.")
	serverCmd.Flags().BoolP("terminal-webgl", "", true, "Enable WebGL terminal renderer.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_WEBGL environment variable if set.")
	serverCmd.Flags().StringP("download-path", "", "", "The path to serve download files from if set.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_DOWNLOAD_PATH environment variable if set.")
	serverCmd.Flags().StringP("wildcard-domain", "", "", "The wildcard domain to use for proxying to spaces.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_WILDCARD_DOMAIN environment variable if set.")
	serverCmd.Flags().StringP("encrypt", "", "", "The encryption key to use for encrypting stored variables.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_ENCRYPT environment variable if set.")
	serverCmd.Flags().StringP("agent-endpoint", "", "", "The address agents should use to talk to the server (default \"\").\nOverrides the "+config.CONFIG_ENV_PREFIX+"_AGENT_ENDPOINT environment variable if set.")
	serverCmd.Flags().StringP("location", "", "", "The location of the server (defaults to NOMAD_DC or hostname).\nOverrides the "+config.CONFIG_ENV_PREFIX+"_LOCATION environment variable if set.")
	serverCmd.Flags().StringP("origin-server", "", "", "The address of the origin server, when given this server becomes a leaf server (default \"\").\nOverrides the "+config.CONFIG_ENV_PREFIX+"_ORIGIN_SERVER environment variable if set.")
	serverCmd.Flags().StringP("shared-token", "", "", "The shared token for lear to origin server communication (default \"\").\nOverrides the "+config.CONFIG_ENV_PREFIX+"_SHARED_TOKEN environment variable if set.")
	serverCmd.Flags().StringP("html-path", "", "", "The optional path to the html files to serve, if not given then then internal files are used.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_HTML_PATH environment variable if set.")
	serverCmd.Flags().StringP("template-path", "", "", "The optional path to the template files to serve, if not given then then internal files are used.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_TEMPLATE_PATH environment variable if set.")
	serverCmd.Flags().StringP("agent-path", "", "", "The optional path to the agent files to serve, if not given then then internal files are used.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_AGENT_PATH environment variable if set.")
	serverCmd.Flags().BoolP("enable-leaf-api-tokens", "", false, "Allow the leaf servers to use an API token for authentication with the origin server.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_ENABLE_LEAF_API_TOKENS environment variable if set.")
	serverCmd.Flags().StringP("timezone", "", "", "The timezone to use for the server (default is system timezone).\nOverrides the "+config.CONFIG_ENV_PREFIX+"_TIMEZONE environment variable if set.")
	serverCmd.Flags().StringP("tunnel-domain", "", "", "The domain to use for tunnel connections.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_TUNNEL_DOMAIN environment variable if set.")
	serverCmd.Flags().Int("audit-retention", 90, "The number of days to keep audit logs (default \"90\").\nOverrides the "+config.CONFIG_ENV_PREFIX+"_AUDIT_RETENTION environment variable if set.")
	serverCmd.Flags().BoolP("disable-space-create", "", false, "Disable the ability to create spaces.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_DISABLE_SPACE_CREATE environment variable if set.")
	serverCmd.Flags().BoolP("hide-support-links", "", false, "Hide the support links in the UI.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_HIDE_SUPPORT_LINKS environment variable if set.")
	serverCmd.Flags().BoolP("hide-api-tokens", "", false, "Hide the API tokens menu item in the UI.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_HIDE_API_TOKENS environment variable if set.")

	// TOTP
	serverCmd.Flags().BoolP("enable-totp", "", false, "Enable TOTP for users.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_ENABLE_TOTP environment variable if set.")
	serverCmd.Flags().IntP("totp-window", "", 1, "The number of time steps (30 seconds) to check for TOTP codes (default \"1\").\nOverrides the "+config.CONFIG_ENV_PREFIX+"_TOTP_WINDOW environment variable if set.")
	serverCmd.Flags().StringP("totp-issuer", "", "Knot", "The issuer to use for TOTP codes (default \"Knot\").\nOverrides the "+config.CONFIG_ENV_PREFIX+"_TOTP_ISSUER environment variable if set.")

	// DNS Server
	serverCmd.Flags().BoolP("enable-dns", "", false, "Experimental. Enable the DNS server.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_ENABLE_DNS environment variable if set.")
	serverCmd.Flags().StringP("dns-listen", "", ":8600", "The address to listen on for DNS requests (default \":8600\").\nOverrides the "+config.CONFIG_ENV_PREFIX+"_DNS_LISTEN environment variable if set.")
	serverCmd.Flags().StringP("dns-domain", "", "knot.internal", "The domain to listen for DNS requests on (default \"knot.internal\").\nOverrides the "+config.CONFIG_ENV_PREFIX+"_DNS_DOMAIN environment variable if set.")
	serverCmd.Flags().Int("dns-ttl", 10, "The TTL in seconds to use for DNS responses (default \"10\").\nOverrides the "+config.CONFIG_ENV_PREFIX+"_DNS_TTL environment variable if set.")

	// TLS
	serverCmd.Flags().StringP("cert-file", "", "", "The file with the PEM encoded certificate to use for the server.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_CERT_FILE environment variable if set.")
	serverCmd.Flags().StringP("key-file", "", "", "The file with the PEM encoded key to use for the server.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_KEY_FILE environment variable if set.")
	serverCmd.Flags().BoolP("use-tls", "", true, "Enable TLS.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_USE_TLS environment variable if set.")
	serverCmd.Flags().BoolP("agent-use-tls", "", true, "Enable TLS when talking to agents.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_AGENT_USE_TLS environment variable if set.")
	serverCmd.Flags().BoolP("tls-skip-verify", "", true, "Skip TLS verification when talking to agents.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_TLS_SKIP_VERIFY environment variable if set.")

	// Nomad
	serverCmd.Flags().StringP("nomad-addr", "", "http://127.0.0.1:4646", "The address of the Nomad server (default \"http://127.0.0.1:4646\").\nOverrides the "+config.CONFIG_ENV_PREFIX+"_NOMAD_ADDR environment variable if set.")
	serverCmd.Flags().StringP("nomad-token", "", "", "The token to use for Nomad API requests.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_NOMAD_TOKEN environment variable if set.")

	// MySQL
	serverCmd.Flags().BoolP("mysql-enabled", "", false, "Enable MySQL database backend.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_MYSQL_ENABLED environment variable if set.")
	serverCmd.Flags().StringP("mysql-host", "", "", "The MySQL host to connect to (default \"localhost\").\nOverrides the "+config.CONFIG_ENV_PREFIX+"_MYSQL_HOST environment variable if set.")
	serverCmd.Flags().IntP("mysql-port", "", 3306, "The MySQL port to connect to (default \"3306\").\nOverrides the "+config.CONFIG_ENV_PREFIX+"_MYSQL_PORT environment variable if set.")
	serverCmd.Flags().StringP("mysql-user", "", "root", "The MySQL user to connect as (default \"root\").\nOverrides the "+config.CONFIG_ENV_PREFIX+"_MYSQL_USER environment variable if set.")
	serverCmd.Flags().StringP("mysql-password", "", "", "The MySQL password to use.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_MYSQL_PASSWORD environment variable if set.")
	serverCmd.Flags().StringP("mysql-database", "", "knot", "The MySQL database to use (default \"knot\").\nOverrides the "+config.CONFIG_ENV_PREFIX+"_MYSQL_DATABASE environment variable if set.")
	serverCmd.Flags().IntP("mysql-connection-max-idle", "", 2, "The maximum number of idle connections in the connection pool (default \"10\").\nOverrides the "+config.CONFIG_ENV_PREFIX+"_MYSQL_CONNECTION_MAX_IDLE environment variable if set.")
	serverCmd.Flags().IntP("mysql-connection-max-open", "", 100, "The maximum number of open connections to the database (default \"100\").\nOverrides the "+config.CONFIG_ENV_PREFIX+"_MYSQL_CONNECTION_MAX_OPEN environment variable if set.")
	serverCmd.Flags().IntP("mysql-connection-max-lifetime", "", 5, "The maximum amount of time in minutes a connection may be reused (default \"5\").\nOverrides the "+config.CONFIG_ENV_PREFIX+"_MYSQL_CONNECTION_MAX_LIFETIME environment variable if set.")

	// BadgerDB
	serverCmd.Flags().BoolP("badgerdb-enabled", "", false, "Enable BadgerDB database backend.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_BADGERDB_ENABLED environment variable if set.")
	serverCmd.Flags().StringP("badgerdb-path", "", "./badger", "The path to the BadgerDB database (default \"./badger\").\nOverrides the "+config.CONFIG_ENV_PREFIX+"_BADGERDB_PATH environment variable if set.")

	// Redis
	serverCmd.Flags().BoolP("redis-enabled", "", false, "Enable Redis database backend.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_REDIS_ENABLED environment variable if set.")
	serverCmd.Flags().StringSliceP("redis-hosts", "", []string{"localhost:6379"}, "The redis server(s), can be specified multiple times (default \"localhost:6379\").\nOverrides the "+config.CONFIG_ENV_PREFIX+"_REDIS_HOSTS environment variable if set.")
	serverCmd.Flags().StringP("redis-password", "", "", "The password to use for the redis server.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_REDIS_PASSWORD environment variable if set.")
	serverCmd.Flags().IntP("redis-db", "", 0, "The redis database to use (default \"0\").\nOverrides the "+config.CONFIG_ENV_PREFIX+"_REDIS_DB environment variable if set.")
	serverCmd.Flags().StringP("redis-master-name", "", "", "The name of the master to use for failover clients (default \"\").\nOverrides the "+config.CONFIG_ENV_PREFIX+"_REDIS_MASTER_NAME environment variable if set.")
	serverCmd.Flags().StringP("redis-key-prefix", "", "", "The prefix to use for all keys in the redis database (default \"\").\nOverrides the "+config.CONFIG_ENV_PREFIX+"_REDIS_KEY_PREFIX environment variable if set.")

	// Memory
	serverCmd.Flags().BoolP("memorydb-enabled", "", false, "Enable memory database backend for session storage.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_MEMORYDB_ENABLED environment variable if set.")

	RootCmd.AddCommand(serverCmd)
}

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the knot server",
	Long:  `Start the knot server and listen for incoming connections.`,
	Args:  cobra.NoArgs,
	PreRun: func(cmd *cobra.Command, args []string) {
		viper.BindPFlag("server.listen", cmd.Flags().Lookup("listen"))
		viper.BindEnv("server.listen", config.CONFIG_ENV_PREFIX+"_LISTEN")
		viper.SetDefault("server.listen", ":3000")

		viper.BindPFlag("server.url", cmd.Flags().Lookup("url"))
		viper.BindEnv("server.url", config.CONFIG_ENV_PREFIX+"_URL")
		viper.SetDefault("server.url", "http://127.0.0.1:3000")

		viper.BindPFlag("server.listen_agent", cmd.Flags().Lookup("listen-agent"))
		viper.BindEnv("server.listen_agent", config.CONFIG_ENV_PREFIX+"_LISTEN_AGENT")
		viper.SetDefault("server.listen_agent", "127.0.0.1:3010")

		viper.BindPFlag("server.listen_tunnel", cmd.Flags().Lookup("listen-tunnel"))
		viper.BindEnv("server.listen_tunnel", config.CONFIG_ENV_PREFIX+"_LISTEN_TUNNEL")
		viper.SetDefault("server.listen_tunnel", "")

		viper.BindPFlag("server.wildcard_domain", cmd.Flags().Lookup("wildcard-domain"))
		viper.BindEnv("server.wildcard_domain", config.CONFIG_ENV_PREFIX+"_WILDCARD_DOMAIN")
		viper.SetDefault("server.wildcard_domain", "")

		viper.BindPFlag("resolver.nameservers", cmd.Flags().Lookup("nameserver"))
		viper.BindEnv("resolver.nameservers", config.CONFIG_ENV_PREFIX+"_NAMESERVERS")

		viper.BindPFlag("server.enable_proxy", cmd.Flags().Lookup("enable-proxy"))
		viper.BindEnv("server.enable_proxy", config.CONFIG_ENV_PREFIX+"_ENABLE_PROXY")
		viper.SetDefault("server.enable_proxy", false)

		viper.BindPFlag("server.terminal.webgl", cmd.Flags().Lookup("terminal-webgl"))
		viper.BindEnv("server.terminal.webgl", config.CONFIG_ENV_PREFIX+"_WEBGL")
		viper.SetDefault("server.terminal.webgl", true)

		viper.BindPFlag("server.download_path", cmd.Flags().Lookup("download-path"))
		viper.BindEnv("server.download_path", config.CONFIG_ENV_PREFIX+"_DOWNLOAD_PATH")
		viper.SetDefault("server.download_path", "")

		viper.BindPFlag("server.html_path", cmd.Flags().Lookup("html-path"))
		viper.BindEnv("server.html_path", config.CONFIG_ENV_PREFIX+"_HTML_PATH")
		viper.SetDefault("server.html_path", "")

		viper.BindPFlag("server.template_path", cmd.Flags().Lookup("template-path"))
		viper.BindEnv("server.template_path", config.CONFIG_ENV_PREFIX+"_TEMPLATE_PATH")
		viper.SetDefault("server.template_path", "")

		viper.BindPFlag("server.agent_path", cmd.Flags().Lookup("agent-path"))
		viper.BindEnv("server.agent_path", config.CONFIG_ENV_PREFIX+"_AGENT_PATH")
		viper.SetDefault("server.agent_path", "")

		viper.BindPFlag("server.encrypt", cmd.Flags().Lookup("encrypt"))
		viper.BindEnv("server.encrypt", config.CONFIG_ENV_PREFIX+"_ENCRYPT")
		viper.SetDefault("server.encrypt", "")

		viper.BindPFlag("server.agent_endpoint", cmd.Flags().Lookup("agent-endpoint"))
		viper.BindEnv("server.agent_endpoint", config.CONFIG_ENV_PREFIX+"_AGENT_ENDPOINT")
		viper.SetDefault("server.agent_endpoint", "")

		viper.BindPFlag("server.enable_leaf_api_tokens", cmd.Flags().Lookup("enable-leaf-api-tokens"))
		viper.BindEnv("server.enable_leaf_api_tokens", config.CONFIG_ENV_PREFIX+"_ENABLE_LEAF_API_TOKENS")
		viper.SetDefault("server.enable_leaf_api_tokens", false)

		viper.BindPFlag("server.ui.hide_support_links", cmd.Flags().Lookup("hide-support-links"))
		viper.BindEnv("server.ui.hide_support_links", config.CONFIG_ENV_PREFIX+"_HIDE_SUPPORT_LINKS")
		viper.SetDefault("server.ui.hide_support_links", false)

		viper.BindPFlag("server.ui.hide_api_tokens", cmd.Flags().Lookup("hide-api-tokens"))
		viper.BindEnv("server.ui.hide_api_tokens", config.CONFIG_ENV_PREFIX+"_HIDE_API_TOKENS")
		viper.SetDefault("server.ui.hide_api_tokens", false)

		// Get the hostname
		hostname := os.Getenv("NOMAD_DC")
		if hostname == "" {
			var err error

			hostname, err = os.Hostname()
			if err != nil {
				log.Fatal().Msgf("Error getting hostname: %v", err)
			}
			hostname = strings.Split(hostname, ".")[0]
		}

		viper.BindPFlag("server.location", cmd.Flags().Lookup("location"))
		viper.BindEnv("server.location", config.CONFIG_ENV_PREFIX+"_LOCATION")
		viper.SetDefault("server.location", hostname)

		viper.BindPFlag("server.origin_server", cmd.Flags().Lookup("origin-server"))
		viper.BindEnv("server.origin_server", config.CONFIG_ENV_PREFIX+"_ORIGIN_SERVER")
		viper.SetDefault("server.origin_server", "")

		viper.BindPFlag("server.shared_token", cmd.Flags().Lookup("shared-token"))
		viper.BindEnv("server.shared_token", config.CONFIG_ENV_PREFIX+"_SHARED_TOKEN")
		viper.SetDefault("server.shared_token", "")

		viper.BindPFlag("server.tunnel_domain", cmd.Flags().Lookup("tunnel-domain"))
		viper.BindEnv("server.tunnel_domain", config.CONFIG_ENV_PREFIX+"_TUNNEL_DOMAIN")
		viper.SetDefault("server.tunnel_domain", "")

		viper.BindPFlag("server.audit_retention", cmd.Flags().Lookup("audit-retention"))
		viper.BindEnv("server.audit_retention", config.CONFIG_ENV_PREFIX+"_AUDIT_RETENTION")
		viper.SetDefault("server.audit_retention", 90)

		viper.BindPFlag("server.disable_space_create", cmd.Flags().Lookup("disable-space-create"))
		viper.BindEnv("server.disable_space_create", config.CONFIG_ENV_PREFIX+"_DISABLE_SPACE_CREATE")
		viper.SetDefault("server.disable_space_create", false)

		// TLS
		viper.BindPFlag("server.tls.cert_file", cmd.Flags().Lookup("cert-file"))
		viper.BindEnv("server.tls.cert_file", config.CONFIG_ENV_PREFIX+"_CERT_FILE")
		viper.SetDefault("server.tls.cert_file", "")

		viper.BindPFlag("server.tls.key_file", cmd.Flags().Lookup("key-file"))
		viper.BindEnv("server.tls.key_file", config.CONFIG_ENV_PREFIX+"_KEY_FILE")
		viper.SetDefault("server.tls.key_file", "")

		viper.BindPFlag("server.tls.use_tls", cmd.Flags().Lookup("use-tls"))
		viper.BindEnv("server.tls.use_tls", config.CONFIG_ENV_PREFIX+"_USE_TLS")
		viper.SetDefault("server.tls.use_tls", true)

		viper.BindPFlag("server.tls.agent_use_tls", cmd.Flags().Lookup("agent-use-tls"))
		viper.BindEnv("server.tls.agent_use_tls", config.CONFIG_ENV_PREFIX+"_AGENT_USE_TLS")
		viper.SetDefault("server.tls.agent_use_tls", true)

		viper.BindPFlag("tls_skip_verify", cmd.Flags().Lookup("tls-skip-verify"))
		viper.BindEnv("tls_skip_verify", config.CONFIG_ENV_PREFIX+"_TLS_SKIP_VERIFY")
		viper.SetDefault("tls_skip_verify", true)

		viper.BindPFlag("server.timezone", cmd.Flags().Lookup("timezone"))
		viper.BindEnv("server.timezone", config.CONFIG_ENV_PREFIX+"_TIMEZONE")
		viper.SetDefault("server.timezone", "")

		// TOTP
		viper.BindPFlag("server.totp.enabled", cmd.Flags().Lookup("enable-totp"))
		viper.BindEnv("server.totp.enabled", config.CONFIG_ENV_PREFIX+"_ENABLE_TOTP")
		viper.SetDefault("server.totp.enabled", false)

		viper.BindPFlag("server.totp.window", cmd.Flags().Lookup("totp-window"))
		viper.BindEnv("server.totp.window", config.CONFIG_ENV_PREFIX+"_TOTP_WINDOW")
		viper.SetDefault("server.totp.window", 1)

		viper.BindPFlag("server.totp.issuer", cmd.Flags().Lookup("totp-issuer"))
		viper.BindEnv("server.totp.issuer", config.CONFIG_ENV_PREFIX+"_TOTP_ISSUER")
		viper.SetDefault("server.totp.issuer", "Knot")

		// DNS
		viper.BindPFlag("server.dns.enabled", cmd.Flags().Lookup("enable-dns"))
		viper.BindEnv("server.dns.enabled", config.CONFIG_ENV_PREFIX+"_ENABLE_DNS")
		viper.SetDefault("server.dns.enabled", false)

		viper.BindPFlag("server.dns.listen", cmd.Flags().Lookup("dns-listen"))
		viper.BindEnv("server.dns.listen", config.CONFIG_ENV_PREFIX+"_DNS_LISTEN")
		viper.SetDefault("server.dns.listen", ":8600")

		viper.BindPFlag("server.dns.domain", cmd.Flags().Lookup("dns-domain"))
		viper.BindEnv("server.dns.domain", config.CONFIG_ENV_PREFIX+"_DNS_DOMAIN")
		viper.SetDefault("server.dns.domain", "knot.internal")

		viper.BindPFlag("server.dns.ttl", cmd.Flags().Lookup("dns-ttl"))
		viper.BindEnv("server.dns.ttl", config.CONFIG_ENV_PREFIX+"_DNS_TTL")
		viper.SetDefault("server.dns.ttl", 10)

		// Nomad
		viper.BindPFlag("server.nomad.addr", cmd.Flags().Lookup("nomad-addr"))
		viper.BindEnv("server.nomad.addr", config.CONFIG_ENV_PREFIX+"_NOMAD_ADDR")
		viper.SetDefault("server.nomad.addr", "http://127.0.0.1:4646")

		viper.BindPFlag("server.nomad.token", cmd.Flags().Lookup("nomad-token"))
		viper.BindEnv("server.nomad.token", config.CONFIG_ENV_PREFIX+"_NOMAD_TOKEN")
		viper.SetDefault("server.nomad.token", "")

		// MySQL
		viper.BindPFlag("server.mysql.enabled", cmd.Flags().Lookup("mysql-enabled"))
		viper.BindEnv("server.mysql.enabled", config.CONFIG_ENV_PREFIX+"_MYSQL_ENABLED")
		viper.SetDefault("server.mysql.enabled", false)
		viper.BindPFlag("server.mysql.host", cmd.Flags().Lookup("mysql-host"))
		viper.BindEnv("server.mysql.host", config.CONFIG_ENV_PREFIX+"_MYSQL_HOST")
		viper.SetDefault("server.mysql.host", "localhost")
		viper.BindPFlag("server.mysql.port", cmd.Flags().Lookup("mysql-port"))
		viper.BindEnv("server.mysql.port", config.CONFIG_ENV_PREFIX+"_MYSQL_PORT")
		viper.SetDefault("server.mysql.port", 3306)
		viper.BindPFlag("server.mysql.user", cmd.Flags().Lookup("mysql-user"))
		viper.BindEnv("server.mysql.user", config.CONFIG_ENV_PREFIX+"_MYSQL_USER")
		viper.SetDefault("server.mysql.user", "root")
		viper.BindPFlag("server.mysql.password", cmd.Flags().Lookup("mysql-password"))
		viper.BindEnv("server.mysql.password", config.CONFIG_ENV_PREFIX+"_MYSQL_PASSWORD")
		viper.SetDefault("server.mysql.password", "")
		viper.BindPFlag("server.mysql.database", cmd.Flags().Lookup("mysql-database"))
		viper.BindEnv("server.mysql.database", config.CONFIG_ENV_PREFIX+"_MYSQL_DATABASE")
		viper.SetDefault("server.mysql.database", "knot")
		viper.BindPFlag("server.mysql.connection_max_idle", cmd.Flags().Lookup("mysql-connection-max-idle"))
		viper.BindEnv("server.mysql.connection_max_idle", config.CONFIG_ENV_PREFIX+"_MYSQL_CONNECTION_MAX_IDLE")
		viper.SetDefault("server.mysql.connection_max_idle", 10)
		viper.BindPFlag("server.mysql.connection_max_open", cmd.Flags().Lookup("mysql-connection-max-open"))
		viper.BindEnv("server.mysql.connection_max_open", config.CONFIG_ENV_PREFIX+"_MYSQL_CONNECTION_MAX_OPEN")
		viper.SetDefault("server.mysql.connection_max_open", 100)
		viper.BindPFlag("server.mysql.connection_max_lifetime", cmd.Flags().Lookup("mysql-connection-max-lifetime"))
		viper.BindEnv("server.mysql.connection_max_lifetime", config.CONFIG_ENV_PREFIX+"_MYSQL_CONNECTION_MAX_LIFETIME")
		viper.SetDefault("server.mysql.connection_max_lifetime", 5)

		// BadgerDB
		viper.BindPFlag("server.badgerdb.enabled", cmd.Flags().Lookup("badgerdb-enabled"))
		viper.BindEnv("server.badgerdb.enabled", config.CONFIG_ENV_PREFIX+"_BADGERDB_ENABLED")
		viper.SetDefault("server.badgerdb.enabled", false)
		viper.BindPFlag("server.badgerdb.path", cmd.Flags().Lookup("badgerdb-path"))
		viper.BindEnv("server.badgerdb.path", config.CONFIG_ENV_PREFIX+"_BADGERDB_PATH")
		viper.SetDefault("server.badgerdb.path", "./badger")

		// Redis
		viper.BindPFlag("server.redis.enabled", cmd.Flags().Lookup("redis-enabled"))
		viper.BindEnv("server.redis.enabled", config.CONFIG_ENV_PREFIX+"_REDIS_ENABLED")
		viper.SetDefault("server.redis.enabled", false)
		viper.BindPFlag("server.redis.hosts", cmd.Flags().Lookup("redis-hosts"))
		viper.BindEnv("server.redis.hosts", config.CONFIG_ENV_PREFIX+"_REDIS_HOSTS")
		viper.SetDefault("server.redis.hosts", []string{"localhost:6379"})
		viper.BindPFlag("server.redis.password", cmd.Flags().Lookup("redis-password"))
		viper.BindEnv("server.redis.password", config.CONFIG_ENV_PREFIX+"_REDIS_PASSWORD")
		viper.SetDefault("server.redis.password", "")
		viper.BindPFlag("server.redis.db", cmd.Flags().Lookup("redis-db"))
		viper.BindEnv("server.redis.db", config.CONFIG_ENV_PREFIX+"_REDIS_DB")
		viper.SetDefault("server.redis.db", 0)
		viper.BindPFlag("server.redis.master_name", cmd.Flags().Lookup("redis-master-name"))
		viper.BindEnv("server.redis.master_name", config.CONFIG_ENV_PREFIX+"_REDIS_MASTER_NAME")
		viper.SetDefault("server.redis.master_name", "")
		viper.BindPFlag("server.redis.key_prefix", cmd.Flags().Lookup("redis-key-prefix"))
		viper.BindEnv("server.redis.key_prefix", config.CONFIG_ENV_PREFIX+"_REDIS_KEY_PREFIX")
		viper.SetDefault("server.redis.key_prefix", "")

		// Memory
		viper.BindPFlag("server.memorydb.enabled", cmd.Flags().Lookup("memorydb-enabled"))
		viper.BindEnv("server.memorydb.enabled", config.CONFIG_ENV_PREFIX+"_MEMORYDB_ENABLED")
		viper.SetDefault("server.memorydb.enabled", false)

		// Set if leaf, origin or standalone server
		if viper.GetString("server.shared_token") != "" {
			server_info.IsLeaf = viper.GetString("server.origin_server") != ""
			server_info.IsOrigin = viper.GetString("server.origin_server") == ""
		}
	},
	Run: func(cmd *cobra.Command, args []string) {
		listen := util.FixListenAddress(viper.GetString("server.listen"))

		// If agent address not given then don't start
		if viper.GetString("server.agent_endpoint") == "" {
			log.Fatal().Msg("server: agent endpoint not given")
		}

		log.Info().Msgf("server: starting knot version: %s", build.Version)
		log.Info().Msgf("server: starting on: %s", listen)

		audit.Log(
			model.AuditActorSystem,
			model.AuditActorTypeSystem,
			model.AuditEventSystemStart,
			"",
			&map[string]interface{}{
				"build": build.Version,
			},
		)

		// If server.tunnel-domain doesn't start with a . then prefix it, strip leading * if present
		tunnelDomain := viper.GetString("server.tunnel_domain")
		if tunnelDomain != "" {
			tunnelDomain = strings.TrimPrefix(tunnelDomain, "*")

			if !strings.HasPrefix(tunnelDomain, ".") {
				tunnelDomain = "." + tunnelDomain
			}

			viper.Set("server.tunnel_domain", tunnelDomain)
		}

		// set the server location and timezone
		server_info.LeafLocation = viper.GetString("server.location")
		server_info.Timezone = viper.GetString("server.timezone")

		if server_info.Timezone == "" {
			server_info.Timezone, _ = time.Now().Zone()
		}
		log.Info().Msgf("server: timezone: %s", server_info.Timezone)

		// Initialize the middleware, test if users are present
		middleware.Initialize()

		// Load template hashes
		api_utils.LoadTemplateHashes()

		// this is a leaf node, connect to the origin server
		if server_info.IsLeaf {
			origin_leaf.LeafConnectAndServe(viper.GetString("server.origin_server"))

			// start keep alive for remote sessions
			remoteSessionKeepAlive()
		} else {
			// Load roles into memory cache
			roles, err := database.GetInstance().GetRoles()
			if err != nil {
				log.Fatal().Msgf("server: failed to get roles: %s", err.Error())
			}
			model.SetRoleCache(roles)

			if server_info.IsOrigin {
				origin_leaf.StartLeafSessionGC()
			}
		}

		// Check for local spaces that are pending state changes and setup watches
		startupCheckPendingSpaces()

		// Start the DNS server
		if viper.GetBool("server.dns.enabled") {
			dnsserver.ListenAndServe()
		}

		var router http.Handler

		// Get the main host domain & wildcard domain
		wildcardDomain := viper.GetString("server.wildcard_domain")
		serverURL := viper.GetString("server.url")
		u, err := url.Parse(serverURL)
		if err != nil {
			log.Fatal().Msg(err.Error())
		}

		log.Debug().Msgf("Host: %s", u.Host)

		// Create the application routes
		routes := http.NewServeMux()

		api.ApiRoutes(routes)
		proxy.Routes(routes)
		web.Routes(routes)

		// Add support for page not found
		appRoutes := web.HandlePageNotFound(routes)

		if viper.GetString("server.listen_tunnel") != "" {
			tunnel_server.Routes(routes)
		}

		// If have a wildcard domain, build it's routes
		if wildcardDomain != "" {
			log.Debug().Msgf("Wildcard Domain: %s", wildcardDomain)

			// Create a regex to match the wildcard domain
			match := regexp.MustCompile("^[a-zA-Z0-9-]+" + strings.TrimLeft(strings.Replace(wildcardDomain, ".", "\\.", -1), "*") + "$")

			// Get the routes for the wildcard domain
			wildcardRoutes := proxy.PortRoutes()

			domainMux := http.NewServeMux()
			domainMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				if r.Host == u.Host {
					appRoutes.ServeHTTP(w, r)
				} else if match.MatchString(r.Host) {
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
				if u.Host != "localhost" {
					sslDomains = append(sslDomains, "localhost")
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

		// Start the agent server
		agent_server.ListenAndServe(util.FixListenAddress(viper.GetString("server.listen_agent")), tlsConfig)

		// Start a tunnel server
		if viper.GetString("server.listen_tunnel") != "" {
			tunnel_server.ListenAndServe(util.FixListenAddress(viper.GetString("server.listen_tunnel")), tlsConfig)
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
					log.Fatal().Msgf("server: %v", err.Error())
				}
			}()
		} else {
			go func() {
				if err := server.ListenAndServe(); err != http.ErrServerClosed {
					log.Fatal().Msgf("server: %v", err.Error())
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
		fmt.Print("\r")
		log.Info().Msg("server: shutdown")
		os.Exit(0)
	},
}

// periodically ping the origin server to keep remote sessions alive
func remoteSessionKeepAlive() {
	log.Info().Msg("server: starting remote server session refresh services")

	// Start a go routine that runs once per hour and pings all sessions to keep them alive
	go func() {
		for {
			time.Sleep(30 * time.Minute)

			log.Debug().Msg("leaf: refreshing remote sessions")

			db := database.GetCacheInstance()
			sessions, err := db.GetSessions()
			if err != nil {
				log.Error().Msgf("failed to get sessions: %s", err.Error())
				continue
			}

			var count int = 0
			for _, session := range sessions {
				if session.RemoteSessionId != "" {
					count++
					client := apiclient.NewRemoteSession(session.RemoteSessionId)
					_, err := client.Ping()
					if err != nil {
						log.Error().Msgf("failed to ping session: %s", err.Error())
					}
				}
			}

			log.Debug().Msgf("leaf: refreshed %d remote sessions", count)
		}
	}()
}

func startupCheckPendingSpaces() {
	log.Info().Msg("server: checking for pending spaces")

	db := database.GetInstance()
	spaces, err := db.GetSpaces()
	if err != nil {
		log.Fatal().Msgf("server: failed to get spaces: %s", err.Error())
	} else {
		nomadClient := nomad.NewClient()

		for _, space := range spaces {
			// If space on this server and pending then monitor it
			if space.Location == server_info.LeafLocation && space.IsPending {
				log.Info().Msgf("server: found pending space %s", space.Name)
				nomadClient.MonitorJobState(space)
			}

			// If deleting then delete it
			if space.IsDeleting && (space.Location == "" || space.Location == server_info.LeafLocation) {
				log.Info().Msgf("server: found deleting space %s", space.Name)
				api.RealDeleteSpace(space)
			}
		}
	}

	log.Info().Msg("server: finished checking for pending spaces")
}
