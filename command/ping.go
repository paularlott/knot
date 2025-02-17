package command

import (
	"fmt"
	"os"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	pingCmd.Flags().StringP("server", "s", "", "The address of the remote server to proxy through.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_SERVER environment variable if set.")

	RootCmd.AddCommand(pingCmd)
}

var pingCmd = &cobra.Command{
	Use:   "ping",
	Short: "Ping the server",
	Long:  `Ping the server and display the health and version number.`,
	Args:  cobra.NoArgs,
	PreRun: func(cmd *cobra.Command, args []string) {
		viper.BindPFlag("client.server", cmd.Flags().Lookup("server"))
		viper.BindEnv("client.server", config.CONFIG_ENV_PREFIX+"_SERVER")
	},
	Run: func(cmd *cobra.Command, args []string) {
		cfg := config.GetServerAddr()
		fmt.Println("Pinging server: ", cfg.HttpServer)

		client := apiclient.NewClient(cfg.HttpServer, cfg.ApiToken, viper.GetBool("tls_skip_verify"))

		version, err := client.Ping()
		if err != nil {
			fmt.Println("Failed to ping server")
			os.Exit(1)
		}

		fmt.Println("\nServer is healthy")
		fmt.Println("Version: ", version)
	},
}
