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
			fmt.Println("Failed to parse server URL")
			os.Exit(1)
		}

		// Get the host name
		hostname, err := os.Hostname()
		if err != nil {
			fmt.Println("Failed to get hostname")
			os.Exit(1)
		}

		hostname = "knot client " + hostname

		client, err := apiclient.NewClient(
			server,
			"",
			cmd.GetBool("tls-skip-verify"),
		)
		if err != nil {
			fmt.Println("Failed to create API client:", err)
			os.Exit(1)
		}

		// Query if the server is using TOTP
		totp, _, err := client.UsingTOTP(context.Background())
		if err != nil {
			fmt.Println("Failed to query server for TOTP")
			os.Exit(1)
		}

		// If using web authentication or server has TOTP enabled then open the server URL in the default browser
		if totp || cmd.GetBool("use-web-auth") {
			u.Path = "/api-tokens/create/" + url.PathEscape(hostname)
			err = util.OpenBrowser(u.String())
			if err != nil {
				fmt.Println("Failed to open server URL, you will need to generate the API token manually")
				os.Exit(1)
			}
			fmt.Print("Enter token: ")
			_, err = fmt.Scanln(&token)
			if err != nil {
				fmt.Println("Failed to read token, you will need to generate the API token manually")
				os.Exit(1)
			}

			// Check the server is compatible before saving the connection
			client.SetAuthToken(token)
			requireCompatibleServer(client)
		} else {
			username := cmd.GetString("username")
			var password []byte

			if username == "" {
				fmt.Print("Enter email: ")
				_, err = fmt.Scanln(&username)
				if err != nil {
					fmt.Println("Failed to read email address")
					os.Exit(1)
				}
			}

			fmt.Print("Enter password: ")
			password, err = term.ReadPassword(int(syscall.Stdin))
			if err != nil {
				fmt.Println("Failed to read password")
				os.Exit(1)
			}
			fmt.Println()

			if username == "" || string(password) == "" {
				fmt.Println("Username and password must be given")
				os.Exit(1)
			}

			response, _, _ := client.Login(context.Background(), username, string(password), "")
			if response == nil || response.Token == "" {
				fmt.Println("Failed to login")
				os.Exit(1)
			}

			client.UseSessionCookie(true).SetAuthToken(response.Token)

			// Refuse servers too old to talk to this client before the token
			// creation fails with an unexplained error. No version reported
			// means a server from before version checking existed.
			requireCompatibleServer(client)

			token, _, err = client.CreateToken(context.Background(), hostname, nil)
			if err != nil || token == "" {
				fmt.Println("Failed to create token")
				os.Exit(1)
			}
		}

		alias := cmd.GetString("alias")
		if err := config.SaveConnection(alias, server, token, cmd); err != nil {
			fmt.Println("Failed to save connection:", err)
			os.Exit(1)
		}

		fmt.Println("Successfully connected to server:", server)
		return nil
	},
}

// requireCompatibleServer checks the authenticated server reports a version
// this client is compatible with, exiting with guidance when it doesn't.
func requireCompatibleServer(client *apiclient.ApiClient) {
	ping, err := client.Ping(context.Background())
	if err != nil {
		fmt.Println("Failed to check server version:", err)
		os.Exit(1)
	}

	if !build.IsCompatible(ping.Version) {
		if ping.Version == "" {
			fmt.Println("Client and server are not compatible, the server did not report a version")
		} else {
			fmt.Printf("Client and server are not compatible, client version %s, server version %s\n", build.Version, ping.Version)
		}
		os.Exit(1)
	}
}
