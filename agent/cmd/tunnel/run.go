package command_tunnel

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/paularlott/knot/command/cmdutil"
	"github.com/paularlott/knot/internal/agentlink"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/tunnel_server"
	"github.com/paularlott/knot/internal/util/validate"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/internal/log"
)

// tunnelBaseFlags are the flags shared by the http and https subcommands. The
// server/token/alias/TLS flags are only used in foreground mode.
func tunnelBaseFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:     "server",
			Aliases:  []string{"s"},
			Usage:    "The address of the remote server to create the tunnel on.",
			EnvVars:  []string{config.CONFIG_ENV_PREFIX + "_SERVER"},
		},
		&cli.StringFlag{
			Name:     "token",
			Aliases:  []string{"t"},
			Usage:    "The token to use for authentication.",
			EnvVars:  []string{config.CONFIG_ENV_PREFIX + "_TOKEN"},
		},
		&cli.BoolFlag{
			Name:         "tls-skip-verify",
			Usage:        "Skip TLS verification when talking to server.",
			ConfigPath:   []string{"tls.skip_verify"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_TLS_SKIP_VERIFY"},
			DefaultValue: true,
		},
		&cli.StringFlag{
			Name:    "port-tls-name",
			Usage:   "The name to present to local port when using.",
			EnvVars: []string{config.CONFIG_ENV_PREFIX + "_TLS_NAME"},
		},
		&cli.BoolFlag{
			Name:         "port-tls-skip-verify",
			Usage:        "Skip TLS verification when talking to local port via https, this allows self signed certificates.",
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_PORT_TLS_SKIP_VERIFY"},
			DefaultValue: true,
		},
		&cli.StringFlag{
			Name:         "alias",
			Aliases:      []string{"a"},
			Usage:        "The server alias to use.",
			DefaultValue: "default",
		},
	}
}

// newWebTunnelCmd builds an http or https web-tunnel subcommand.
//
// withDaemon is true only in the agent binary, where a --daemon flag is offered
// that hands the tunnel to the knot agent. The desktop (main) binary passes
// false, giving foreground-only operation — there is no agent to own a daemon
// tunnel on a workstation.
func newWebTunnelCmd(name, protocol, description string, withDaemon bool) *cli.Command {
	flags := tunnelBaseFlags()
	if withDaemon {
		flags = append(flags, &cli.BoolFlag{
			Name:  "daemon",
			Usage: "Hand the tunnel to the knot agent and exit. The tunnel then lives for the life of the agent.",
		})
	}

	return &cli.Command{
		Name:        name,
		Usage:       fmt.Sprintf("Open an %s tunnel", name),
		Description: description,
		Arguments: []cli.Argument{
			&cli.IntArg{
				Name:     "port",
				Usage:    "The local port to tunnel to",
				Required: true,
			},
			&cli.StringArg{
				Name:     "name",
				Usage:    "The name to expose the tunnel as",
				Required: true,
			},
		},
		MaxArgs: cli.NoArgs,
		Flags:   flags,
		Run: func(ctx context.Context, cmd *cli.Command) error {
			port := cmd.GetIntArg("port")
			if port < 1 || port > 65535 {
				return fmt.Errorf("Invalid port number, port numbers must be between 1 and 65535")
			}

			tunnelName := cmd.GetStringArg("name")
			if !validate.Name(tunnelName) {
				return fmt.Errorf("Invalid name, must be all lowercase and only contain letters, numbers and dashes")
			}

			if withDaemon && cmd.GetBool("daemon") {
				return startDaemonTunnel(protocol, uint16(port), tunnelName, cmd)
			}

			return runForegroundTunnel(ctx, cmd, protocol, uint16(port), tunnelName)
		},
	}
}

// Agent-only subcommands (offer --daemon).
var (
	HttpTunnelCmd = newWebTunnelCmd(
		"http", "http",
		"Open an HTTP tunnel exposing a local port on the internet as <user>--<name>.<domain>.",
		true,
	)
	HttpsTunnelCmd = newWebTunnelCmd(
		"https", "https",
		"Open an HTTPS tunnel exposing a local port on the internet as <user>--<name>.<domain>.",
		true,
	)
)

// Desktop subcommands (foreground only — no --daemon, no agent ownership).
var (
	DesktopHttpTunnelCmd = newWebTunnelCmd(
		"http", "http",
		"Open an HTTP tunnel exposing a local port on the internet as <user>--<name>.<domain>.",
		false,
	)
	DesktopHttpsTunnelCmd = newWebTunnelCmd(
		"https", "https",
		"Open an HTTPS tunnel exposing a local port on the internet as <user>--<name>.<domain>.",
		false,
	)
)

// runForegroundTunnel opens a foreground web tunnel in this process and blocks
// until Ctrl-C. Shared by the agent and desktop binaries.
func runForegroundTunnel(ctx context.Context, cmd *cli.Command, protocol string, port uint16, name string) error {
	cfg := cmdutil.GetServerAddr(cmd)
	if cfg == nil {
		return fmt.Errorf("no server configured")
	}
	client := tunnel_server.NewTunnelClient(
		cfg.WsServer,
		cfg.HttpServer,
		cfg.ApiToken,
		cmd.GetBool("tls-skip-verify"),
		&tunnel_server.TunnelOpts{
			Type:          tunnel_server.WebTunnel,
			Protocol:      protocol,
			LocalPort:     port,
			TunnelName:    name,
			TlsName:       cmd.GetString("port-tls-name"),
			TlsSkipVerify: cmd.GetBool("port-tls-skip-verify"),
		},
	)
	if err := client.ConnectAndServe(); err != nil {
		log.Fatal("Failed to create tunnel", "error", err)
	}

	// Wait for ctrl-c
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	// Block until we receive ctrl-c or tunnel context is cancelled
	select {
	case <-client.GetCtx().Done():
	case <-c:
	}

	client.Shutdown()

	fmt.Println("\r")
	log.Info("Tunnel shutdown")

	return nil
}

func startDaemonTunnel(protocol string, port uint16, name string, cmd *cli.Command) error {
	if !agentlink.IsAgentRunning() {
		return fmt.Errorf("agent not running, --daemon requires the knot agent to be running")
	}

	request := agentlink.StartTunnelRequest{
		Protocol:      protocol,
		Port:          port,
		Name:          name,
		TlsName:       cmd.GetString("port-tls-name"),
		TlsSkipVerify: cmd.GetBool("port-tls-skip-verify"),
	}

	var response agentlink.StartTunnelResponse
	if err := agentlink.SendWithResponseMsg(agentlink.CommandStartTunnel, &request, &response); err != nil {
		return fmt.Errorf("failed to start tunnel: %w", err)
	}

	if !response.Success {
		return fmt.Errorf("%s", response.Error)
	}

	fmt.Printf("Tunnel URL: %s\n", response.URL)
	fmt.Println("Tunnel running in agent (daemon mode).")
	return nil
}
