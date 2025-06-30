package command

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"

	connectcmd "github.com/paularlott/knot/agent/cmd/connect"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"

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
			Name:       "use-web-auth",
			Usage:      "If given then authorization will be done via the web interface.",
			ConfigPath: []string{"use_web_auth"},
			// No EnvVars or DefaultValue in original, add if needed
		},
		&cli.BoolFlag{
			Name:         "tls-skip-verify",
			Usage:        "Skip TLS verification when talking to server.",
			ConfigPath:   []string{"tls_skip_verify"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_TLS_SKIP_VERIFY"},
			DefaultValue: true,
		},
		&cli.StringFlag{
			Name:       "username",
			Aliases:    []string{"u"},
			Usage:      "Username to use for authentication.",
			ConfigPath: []string{"username"},
			EnvVars:    []string{config.CONFIG_ENV_PREFIX + "_USERNAME"},
		},
		&cli.StringFlag{
			Name:         "alias",
			Aliases:      []string{"a"},
			Usage:        "The server alias to use.",
			ConfigPath:   []string{"alias"},
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
			err = open(u.String())
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

			token, _, err = client.CreateToken(context.Background(), hostname)
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

// open opens the specified URL in the default browser of the user.
// https://stackoverflow.com/questions/39320371/how-start-web-server-to-open-page-in-browser-in-golang
func open(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	case "darwin":
		cmd = "open"
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
	}
	args = append(args, url)
	return exec.Command(cmd, args...).Start()
}
