package command_spaces

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/paularlott/cli"
	"github.com/paularlott/knot/command/cmdutil"
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
		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return fmt.Errorf("Failed to create API client: %w", err)
		}

		// Get server info from client
		baseURL := client.GetBaseURL()
		token := client.GetAuthToken()
		wsURL := "ws" + baseURL[4:] + fmt.Sprintf("/logs/%s/stream", spaceName)
		header := http.Header{"Authorization": []string{fmt.Sprintf("Bearer %s", token)}}

		// Connect to the websocket at /logs/<spaceId>/stream and print the logs
		dialer := websocket.DefaultDialer
		dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: cmd.GetBool("tls-skip-verify")}
		dialer.HandshakeTimeout = 5 * time.Second
		ws, response, err := dialer.Dial(wsURL, header)
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
