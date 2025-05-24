package command

import (
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

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func init() {

	ConnectCmd.Flags().BoolP("use-web-auth", "", false, "If given then authorization will be done via the web interface.")
	ConnectCmd.Flags().BoolP("tls-skip-verify", "", true, "Skip TLS verification when talking to server.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_TLS_SKIP_VERIFY environment variable if set.")
	ConnectCmd.Flags().StringP("username", "u", "", "Username to use for authentication.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_USERNAME environment variable if set.")
	ConnectCmd.Flags().StringP("alias", "a", "default", "The server alias to use.")

	RootCmd.AddCommand(ConnectCmd)
	ConnectCmd.AddCommand(connectcmd.ConnectListCmd)
	ConnectCmd.AddCommand(connectcmd.ConnectDeleteCmd)
}

var ConnectCmd = &cobra.Command{
	Use:   "connect <server>",
	Short: "Connect to a server",
	Long:  `Authenticate the client with a remote server and save the server address and access key.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var token string

		server := args[0]

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

		client := apiclient.NewClient(server, "", cmd.Flags().Lookup("tls-skip-verify").Value.String() == "true")

		// Query if the server is using TOTP
		totp, _, err := client.UsingTOTP()
		if err != nil {
			fmt.Println("Failed to query server for TOTP")
			os.Exit(1)
		}

		// If using web authentication or server has TOTP enabled then open the server URL in the default browser
		if totp || cmd.Flags().Lookup("use-web-auth").Value.String() == "true" {

			// Build the registration URL
			u.Path = "/api-tokens/create/" + url.PathEscape(hostname)

			// Open the server URL in the default browser
			err = open(u.String())
			if err != nil {
				fmt.Println("Failed to open server URL, you will need to generate the API token manually")
				os.Exit(1)
			}

			// Accept a string from the user and save it in the variable token
			fmt.Print("Enter token: ")
			_, err = fmt.Scanln(&token)
			if err != nil {
				fmt.Println("Failed to read token, you will need to generate the API token manually")
				os.Exit(1)
			}
		} else {
			var username string = cmd.Flags().Lookup("username").Value.String()
			var password []byte
			var err error

			// If username not given then prompt for it
			if username == "" {

				// Prompt the user to enter their username
				fmt.Print("Enter email: ")
				_, err = fmt.Scanln(&username)
				if err != nil {
					fmt.Println("Failed to read email address")
					os.Exit(1)
				}
			}

			// Prompt the user to enter their password
			fmt.Print("Enter password: ")
			password, err = term.ReadPassword(int(syscall.Stdin))
			if err != nil {
				fmt.Println("Failed to read password")
				os.Exit(1)
			}
			fmt.Println()

			// Check username and password given
			if username == "" || string(password) == "" {
				fmt.Println("Username and password must be given")
				os.Exit(1)
			}

			// Open an API connection to the server
			sessionToken, _, _, _ := client.Login(username, string(password), "")
			if sessionToken == "" {
				fmt.Println("Failed to login")
				os.Exit(1)
			}

			// Use the session token for future requests
			client.UseSessionCookie(true).SetAuthToken(sessionToken)

			// Create an API token
			token, _, err = client.CreateToken(hostname)
			if err != nil || token == "" {
				fmt.Println("Failed to create token")
				os.Exit(1)
			}
		}

		alias, _ := cmd.Flags().GetString("alias")
		if err := config.SaveConnection(alias, server, token); err != nil {
			fmt.Println("Failed to save connection:", err)
			os.Exit(1)
		}

		fmt.Println("Successfully connected to server:", server)
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
