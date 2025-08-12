package command_spaces

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"

	"github.com/gorilla/websocket"
	"github.com/paularlott/cli"
)

var RunCmd = &cli.Command{
	Name:        "run",
	Usage:       "Run a command in a space",
	Description: "Execute a command within a running space and stream the output.",
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:         "timeout",
			Aliases:      []string{"t"},
			Usage:        "Command timeout in seconds",
			DefaultValue: 30,
		},
		&cli.StringFlag{
			Name:         "workdir",
			Aliases:      []string{"w"},
			Usage:        "Working directory for the command",
			DefaultValue: "",
		},
	},
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "space",
			Required: true,
			Usage:    "The name of the space to run the command in",
		},
		&cli.StringArg{
			Name:     "command",
			Required: true,
			Usage:    "The command to run in the space",
		},
	},
	MinArgs: 1,
	MaxArgs: cli.UnlimitedArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		timeout := cmd.GetInt("timeout")
		workdir := cmd.GetString("workdir")
		spaceName := cmd.GetStringArg("space")
		command := cmd.GetStringArg("command")

		// Create a new websocket connection
		alias := cmd.GetString("alias")
		cfg := config.GetServerAddr(alias, cmd)
		client, err := apiclient.NewClient(cfg.HttpServer, cfg.ApiToken, cmd.GetBool("tls-skip-verify"))
		if err != nil {
			return fmt.Errorf("Failed to create API client: %w", err)
		}

		// Get the current user
		user, err := client.WhoAmI(context.Background())
		if err != nil {
			return fmt.Errorf("Error getting user: %w", err)
		}

		// Get a list of available spaces
		spaces, _, err := client.GetSpaces(context.Background(), user.Id)
		if err != nil {
			return fmt.Errorf("Error getting spaces: %w", err)
		}

		// Find the space by name
		var spaceId string
		for _, space := range spaces.Spaces {
			if space.Name == spaceName {
				spaceId = space.Id
				break
			}
		}

		if spaceId == "" {
			return fmt.Errorf("Space not found: %s", spaceName)
		}

		// Connect to the websocket for command execution (new path under /space-io)
		wsUrl := fmt.Sprintf("%s/space-io/%s/run", cfg.WsServer, spaceId)
		header := http.Header{
			"Authorization": []string{fmt.Sprintf("Bearer %s", cfg.ApiToken)},
		}

		dialer := websocket.DefaultDialer
		dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: cmd.GetBool("tls-skip-verify")}
		dialer.HandshakeTimeout = 5 * time.Second
		ws, response, err := dialer.Dial(wsUrl, header)
		if err != nil {
			if response != nil && response.StatusCode == http.StatusUnauthorized {
				return fmt.Errorf("failed to authenticate with server, check remote token")
			} else if response != nil && response.StatusCode == http.StatusForbidden {
				return fmt.Errorf("no permission to run commands in this space")
			}
			return fmt.Errorf("Error connecting to websocket: %w", err)
		}
		defer ws.Close()

		// Send the command execution request as command string plus arguments array
		execRequest := apiclient.RunCommandRequest{
			Command: command,
			Args:    cmd.GetArgs(),
			Timeout: timeout,
			Workdir: workdir,
		}

		err = ws.WriteJSON(execRequest)
		if err != nil {
			return fmt.Errorf("Error sending command: %w", err)
		}

		// Read and display the output
		for {
			_, message, err := ws.ReadMessage()
			if err != nil {
				// Check if this is a normal close
				if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					break
				}
				return fmt.Errorf("Error reading message: %w", err)
			}

			// Check for end of execution marker
			if len(message) == 1 && message[0] == 0 {
				break
			}

			fmt.Print(string(message))
		}

		return nil
	},
}
