package command

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	connectcmd "github.com/paularlott/knot/agent/cmd/connect"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/util"

	"github.com/paularlott/cli"
)

var ConnectCmd = &cli.Command{
	Name:  "connect",
	Usage: "Connect to server",
	Description: "Authenticate the client with a remote server and save the server address and access key.\n\n" +
		"Shows a code and opens the server's web interface, where you sign in and approve the code; " +
		"this works from any computer or phone, so it also works over SSH.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "server",
			Usage:    "The server to connect to",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "no-browser",
			Usage: "Don't open a browser; show the address to visit instead. A browser is never opened over SSH or without a graphical session.",
		},
		&cli.BoolFlag{
			Name:         "tls-skip-verify",
			Usage:        "Skip TLS verification when talking to server.",
			ConfigPath:   []string{"tls.skip_verify"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_TLS_SKIP_VERIFY"},
			DefaultValue: true,
			Global:       true,
		},
		&cli.StringFlag{
			Name:         "alias",
			Aliases:      []string{"a"},
			Usage:        "The server alias to use to identify the connection.",
			DefaultValue: "default",
		},
	},
	Commands: []*cli.Command{
		connectcmd.ConnectListCmd,
		connectcmd.ConnectDeleteCmd,
	},
	Run: func(ctx context.Context, cmd *cli.Command) error {
		server := cmd.GetStringArg("server")

		// If server doesn't start with http or https, assume https
		if !strings.HasPrefix(server, "http://") && !strings.HasPrefix(server, "https://") {
			server = "https://" + server
		}
		if _, err := url.Parse(server); err != nil {
			return fmt.Errorf("invalid server address %q: %w", server, err)
		}

		fmt.Println("Connecting to server: ", server)

		// Get the host name
		hostname, err := os.Hostname()
		if err != nil {
			return fmt.Errorf("couldn't get this computer's host name to name the token: %w", err)
		}

		hostname = "knot client " + hostname

		skipVerify := cmd.GetBool("tls-skip-verify")
		device := apiclient.NewDeviceClient(server, skipVerify)
		auth, err := device.Start(ctx, hostname)
		if errors.Is(err, apiclient.ErrDeviceGrantUnsupported) {
			return fmt.Errorf("%s doesn't support signing in with knot connect; upgrade the server to match this client (version %s)", server, build.Version)
		}
		if err != nil {
			return fmt.Errorf("couldn't connect to %s: %w", server, err)
		}

		fmt.Println()
		fmt.Println("To sign in, visit:", auth.VerificationURI)
		fmt.Println("and enter the code:", auth.UserCode)
		fmt.Println()
		if !cmd.GetBool("no-browser") && auth.VerificationURIComplete != "" && util.CanOpenBrowser() {
			if util.OpenBrowser(auth.VerificationURIComplete) == nil {
				fmt.Println("A browser has been opened; check it shows the same code before approving.")
			}
		}
		fmt.Println("Waiting for approval...")

		token, err := device.Wait(ctx, auth)
		if err != nil {
			return fmt.Errorf("couldn't sign in to %s: %w", server, err)
		}

		// Refuse servers too old to talk to this client before saving the
		// connection.
		client, err := apiclient.NewClient(server, token, skipVerify)
		if err != nil {
			return fmt.Errorf("couldn't create the API client for %s: %w", server, err)
		}
		if err := requireCompatibleServer(client); err != nil {
			return err
		}

		alias := cmd.GetString("alias")
		if err := config.SaveConnection(alias, server, token, cmd); err != nil {
			return fmt.Errorf("couldn't save the connection %q: %w", alias, err)
		}

		fmt.Println("Successfully connected to server:", server)
		return nil
	},
}

// requireCompatibleServer checks the authenticated server reports a version
// this client is compatible with, returning guidance when it doesn't.
func requireCompatibleServer(client *apiclient.ApiClient) error {
	ping, err := client.Ping(context.Background())
	if err != nil {
		return fmt.Errorf("couldn't check the server version: %w", err)
	}

	if !build.IsCompatible(ping.Version) {
		if ping.Version == "" {
			return fmt.Errorf("this client (version %s) isn't compatible with the server, which didn't report a version; upgrade the server or use an older client", build.Version)
		}
		return fmt.Errorf("this client (version %s) isn't compatible with the server (version %s); use a client that matches the server version", build.Version, ping.Version)
	}
	return nil
}
