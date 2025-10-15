package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/dns"
	"github.com/paularlott/knot/internal/log"

	"github.com/paularlott/cli"
	cli_toml "github.com/paularlott/cli/toml"
)

func main() {
	// Logger will be configured with proper level from CLI flags
	log.Configure("info", "console", os.Stderr)

	var configFile = config.CONFIG_FILE

	cmd := &cli.Command{
		Name:        "knot-dns",
		Usage:       "Simple DNS server",
		Description: `knot-dns is a simple DNS server that can forward requests to specific upstream DNS servers based on the domain name.`,
		Version:     build.Version,
		ConfigFile: cli_toml.NewConfigFile(&configFile, func() []string {
			paths := []string{"."}

			home, err := os.UserHomeDir()
			if err == nil {
				paths = append(paths, home)
			}

			paths = append(paths, filepath.Join(home, ".config", config.CONFIG_DIR))

			return paths
		}),
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "config",
				Aliases:     []string{"c"},
				Usage:       "Name and path to the configuration file to use.",
				DefaultText: config.CONFIG_FILE + " in the current directory, $HOME/ or $HOME/.config/" + config.CONFIG_DIR + "/" + config.CONFIG_FILE,
				EnvVars:     []string{config.CONFIG_ENV_PREFIX + "_CONFIG"},
				AssignTo:    &configFile,
			},
			&cli.StringFlag{
				Name:         "log-level",
				Usage:        "Log level one of trace, debug, info, warn, error, fatal, panic",
				ConfigPath:   []string{"log.level"},
				EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_LOGLEVEL"},
				DefaultValue: "info",
			},
			&cli.StringSliceFlag{
				Name:       "nameservers",
				Usage:      "Nameservers to use for DNS resolution, maybe given multiple times.",
				ConfigPath: []string{"resolver.nameservers"},
				EnvVars:    []string{config.CONFIG_ENV_PREFIX + "_NAMESERVERS"},
			},
			// DNS flags
			&cli.BoolFlag{
				Name:         "dns-enabled",
				Usage:        "Enable DNS server.",
				ConfigPath:   []string{"server.dns.enabled"},
				EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_DNS_ENABLED"},
				DefaultValue: true,
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
		PreRun: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			config.InitCommonConfig(cmd)
			return ctx, nil
		},
		Run: func(ctx context.Context, cmd *cli.Command) error {
			if !cmd.GetBool("dns-enabled") {
				return nil
			}

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
				log.Fatal("Failed to create DNS server", "error", err)
			}

			c := make(chan os.Signal, 1)
			signal.Notify(c, os.Interrupt, syscall.SIGTERM)

			if err = dnsServer.Start(); err != nil {
				log.Fatal("Failed to start DNS server", "error", err)
			}
			defer dnsServer.Stop()

			// Block until we receive our signal.
			<-c

			return nil
		},
	}

	err := cmd.Execute(context.Background())
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	os.Exit(0)
}
