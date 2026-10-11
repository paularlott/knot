package command

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"syscall"

	connectcmd "github.com/paularlott/knot/agent/cmd/connect"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/util"

	"github.com/paularlott/cli"
	"golang.org/x/term"
)

var ConnectCmd = &cli.Command{
	Name:        "connect",
	Usage:       "Connect to server",
	Description: "Authenticate the client with a remote server and save the server address and access key.",
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
			Name:  "use-web-auth",
			Usage: "If given then authorization will be done via the web interface.",
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
			Name:    "username",
			Aliases: []string{"u"},
			Usage:   "Username to use for authentication.",
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
		var token string

		server := cmd.GetStringArg("server")

		// If server doesn't start with http or https, assume https
		if !strings.HasPrefix(server, "http://") && !strings.HasPrefix(server, "https://") {
			server = "https://" + server
		}

		fmt.Println("Connecting to server: ", server)

		u, err := url.Parse(server)
		if err != nil {
			return fmt.Errorf("invalid server address %q: %w", server, err)
		}

		// Get the host name
		hostname, err := os.Hostname()
		if err != nil {
			return fmt.Errorf("couldn't get this computer's host name to name the token: %w", err)
		}

		hostname = "knot client " + hostname

		client, err := apiclient.NewClient(
			server,
			"",
			cmd.GetBool("tls-skip-verify"),
		)
		if err != nil {
			return fmt.Errorf("couldn't create the API client for %s: %w", server, err)
		}

		// Query if the server is using TOTP
		totp, _, err := client.UsingTOTP(context.Background())
		if err != nil {
			return fmt.Errorf("couldn't connect to %s: %w", server, err)
		}

		// If using web authentication or server has TOTP enabled then open the server URL in the default browser
		if totp || cmd.GetBool("use-web-auth") {
			u.Path = "/api-tokens/create/" + url.PathEscape(hostname)
			err = util.OpenBrowser(u.String())
			if err != nil {
				return fmt.Errorf("couldn't open %s in a browser (%w); create an API token in the web interface and run connect again", u.String(), err)
			}
			fmt.Print("Enter token: ")
			_, err = fmt.Scanln(&token)
			if err != nil {
				return fmt.Errorf("couldn't read the token: %w", err)
			}

			// Check the server is compatible before saving the connection
			client.SetAuthToken(token)
			if err := requireCompatibleServer(client); err != nil {
				return err
			}
		} else {
			username := cmd.GetString("username")
			var password []byte

			if username == "" {
				fmt.Print("Enter email: ")
				_, err = fmt.Scanln(&username)
				if err != nil {
					return fmt.Errorf("couldn't read the email address: %w", err)
				}
			}

			fmt.Print("Enter password: ")
			password, err = term.ReadPassword(int(syscall.Stdin))
			if err != nil {
				return fmt.Errorf("couldn't read the password: %w", err)
			}
			fmt.Println()

			if username == "" || string(password) == "" {
				return fmt.Errorf("an email address and password are required")
			}

			response, _, err := client.Login(context.Background(), username, string(password), "")
			if err != nil {
				// A 401 here means wrong credentials, not an expired
				// session, so show the server's reason without the
				// "run knot connect" hint the formatter would add.
				if he := apiclient.AsHTTPError(err); he != nil && apiclient.IsUnauthorized(err) {
					return fmt.Errorf("couldn't sign in to %s as %s: %s", server, username, he.Message())
				}
				return fmt.Errorf("couldn't sign in to %s as %s: %w", server, username, err)
			}
			if response == nil || response.Token == "" {
				return fmt.Errorf("couldn't sign in to %s as %s: the server returned no session", server, username)
			}

			client.UseSessionCookie(true).SetAuthToken(response.Token)

			// Refuse servers too old to talk to this client before the token
			// creation fails with an unexplained error. No version reported
			// means a server from before version checking existed.
			if err := requireCompatibleServer(client); err != nil {
				return err
			}

			token, _, err = client.CreateToken(context.Background(), hostname, nil)
			if err != nil {
				return fmt.Errorf("couldn't create an API token on %s: %w", server, err)
			}
			if token == "" {
				return fmt.Errorf("couldn't create an API token on %s: the server returned an empty token", server)
			}
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
