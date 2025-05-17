package space

import (
	"fmt"
	"net/http"
	"os"

	"github.com/paularlott/knot/internal/agent_service_api"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/util/rest"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	SpaceNoteCmd.Flags().IntP("api-port", "", 12201, "The port the agent is listen on for API requests.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_API_PORT environment variable if set.")
}

var SpaceNoteCmd = &cobra.Command{
	Use:   `set-note <note>`,
	Short: "Set the runtime note of the space",
	Long:  `Allows a note to be written for the space which is shown on the dashboard along with the user entered description.`,
	Args:  cobra.ExactArgs(1),
	PreRun: func(cmd *cobra.Command, args []string) {
		viper.BindPFlag("agent.api_port", cmd.Flags().Lookup("api-port"))
		viper.BindEnv("agent.api_port", config.CONFIG_ENV_PREFIX+"_LOGS_PORT")
		viper.SetDefault("agent.api_port", 12201)
	},
	Run: func(cmd *cobra.Command, args []string) {
		client := rest.NewClient("http://127.0.0.1:"+fmt.Sprint(viper.GetInt("agent.api_port")), "", true)

		_, err := client.SendData(http.MethodPost, "/api/space/note", agent_service_api.SpaceNote{
			Note: args[0],
		}, nil, 200)
		if err != nil {
			fmt.Println("Error setting space note: ", err)
			os.Exit(1)
		}

		fmt.Println("Space note set.")
	},
}
