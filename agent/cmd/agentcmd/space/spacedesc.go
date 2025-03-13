package space

import (
	"fmt"
	"net/http"
	"os"

	"github.com/paularlott/knot/internal/agent_service_api"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/util/rest"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	SpaceDescriptionCmd.Flags().IntP("api-port", "", 12201, "The port the agent is listen on for API requests.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_API_PORT environment variable if set.")
}

var SpaceDescriptionCmd = &cobra.Command{
	Use:   `set-description <description>`,
	Short: "Set the description of the space",
	Long:  `Allows the description of the space to be set from the agent command line, this is useful for allowing the description to be set from a script running within the space.`,
	Args:  cobra.ExactArgs(1),
	PreRun: func(cmd *cobra.Command, args []string) {
		viper.BindPFlag("agent.api_port", cmd.Flags().Lookup("api-port"))
		viper.BindEnv("agent.api_port", config.CONFIG_ENV_PREFIX+"_LOGS_PORT")
		viper.SetDefault("agent.api_port", 12201)
	},
	Run: func(cmd *cobra.Command, args []string) {
		client := rest.NewClient("http://127.0.0.1:"+fmt.Sprint(viper.GetInt("agent.api_port")), "", true)

		_, err := client.SendData(http.MethodPost, "/api/space/description", agent_service_api.SpaceDescription{
			Message: args[0],
		}, nil, 200)
		if err != nil {
			fmt.Println("Error setting space description: ", err)
			os.Exit(1)
		}

		fmt.Println("Space description set.")
	},
}
