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

var LogsCmd = &cli.Command{
	Name:        "logs",
	Usage:       "Show the logs from a space",
	Description: "Display the logs for a space.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "space",
			Usage:    "The name of the space to show logs for",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:         "follow",
			Aliases:      []string{"f"},
			Usage:        "Follow the logs.",
			DefaultValue: false,
		},
	},
	Run: func(ctx context.Context, cmd *cli.Command) error {
		spaceName := cmd.GetStringArg("space")
		follow := cmd.GetBool("follow")
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

		// Connect to the websocket at /logs/<spaceId>/stream and print the logs
		wsUrl := fmt.Sprintf("%s/logs/%s/stream", cfg.WsServer, spaceId)
		header := http.Header{"Authorization": []string{fmt.Sprintf("Bearer %s", cfg.ApiToken)}}
		dialer := websocket.DefaultDialer
		dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: cmd.GetBool("tls-skip-verify")}
		dialer.HandshakeTimeout = 5 * time.Second
		ws, response, err := dialer.Dial(wsUrl, header)
		if err != nil {
			if response != nil && response.StatusCode == http.StatusUnauthorized {
				return fmt.Errorf("failed to authenticate with server, check remote token")
			} else if response != nil && response.StatusCode == http.StatusForbidden {
				return fmt.Errorf("no permission to view logs")
			}
			return fmt.Errorf("Error connecting to websocket: %w", err)
		}
		defer ws.Close()

		for {
			_, message, err := ws.ReadMessage()
			if err != nil {
				fmt.Println("Error reading message: ", err)
				break
			}

			// if message is just a byte of 0 then end of history
			if len(message) == 1 && message[0] == 0 && !follow {
				break
			}

			fmt.Print(string(message))
		}
		return nil
	},
}
