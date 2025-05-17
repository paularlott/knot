package command_spaces

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"

	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	logsCmd.Flags().BoolP("follow", "f", false, "Follow the logs.")
}

var logsCmd = &cobra.Command{
	Use:   "logs <space> [flags]",
	Short: "Space logs",
	Long:  `Display the logs for a space.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Fetching logs for space: ", args[0])

		follow, _ := cmd.Flags().GetBool("follow")

		alias, _ := cmd.Flags().GetString("alias")
		cfg := config.GetServerAddr(alias)
		client := apiclient.NewClient(cfg.HttpServer, cfg.ApiToken, viper.GetBool("tls_skip_verify"))

		// Get the current user
		user, err := client.WhoAmI()
		if err != nil {
			fmt.Println("Error getting user: ", err)
			return
		}

		// Get a list of available spaces
		spaces, _, err := client.GetSpaces(user.Id)
		if err != nil {
			fmt.Println("Error getting spaces: ", err)
			return
		}

		// Find the space by name
		var spaceId string = ""
		for _, space := range spaces.Spaces {
			if space.Name == args[0] {
				spaceId = space.Id
				break
			}
		}

		if spaceId == "" {
			fmt.Println("Space not found: ", args[0])
			return
		}

		// Connect to the websocket at /logs/<spaceId>/stream and print the logs
		wsUrl := fmt.Sprintf("%s/logs/%s/stream", cfg.WsServer, spaceId)

		header := http.Header{"Authorization": []string{fmt.Sprintf("Bearer %s", cfg.ApiToken)}}
		dialer := websocket.DefaultDialer
		dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: viper.GetBool("tls_skip_verify")}
		dialer.HandshakeTimeout = 5 * time.Second
		ws, response, err := dialer.Dial(wsUrl, header)
		if err != nil {
			if response != nil && response.StatusCode == http.StatusUnauthorized {
				fmt.Println("failed to authenticate with server, check remote token")
			}

			fmt.Println("Error connecting to websocket: ", err)
			return
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
	},
}
