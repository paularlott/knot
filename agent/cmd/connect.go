package agentcmd

import (
	"fmt"
	"os"

	connectcmd "github.com/paularlott/knot/agent/cmd/connect"
	"github.com/paularlott/knot/internal/agentlink"
	"github.com/paularlott/knot/internal/config"

	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(ConnectCmd)
	ConnectCmd.AddCommand(connectcmd.ConnectListCmd)
	ConnectCmd.AddCommand(connectcmd.ConnectDeleteCmd)
}

var ConnectCmd = &cobra.Command{
	Use:   `connect`,
	Short: "Generate API key",
	Long:  `Asks the running agent to generate a new API key on the server and stores it in the local config.`,
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		var response agentlink.ConnectResponse

		err := agentlink.SendWithResponseMsg(agentlink.CommandConnect, nil, &response)
		if err != nil {
			fmt.Println("Unable to connect to the agent, please check that the agent is running.")
			os.Exit(1)
		}

		if !response.Success {
			fmt.Println("Failed to create an API token")
			os.Exit(1)
		}

		if err := config.SaveConnection("default", response.Server, response.Token); err != nil {
			fmt.Println("Failed to save connection details:", err)
			os.Exit(1)
		}

		fmt.Println("Successfully connected to server:", response.Server)
	},
}
