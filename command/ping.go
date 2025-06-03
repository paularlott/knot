package command

import (
	"context"
	"fmt"
	"os"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	pingCmd.Flags().StringP("server", "s", "", "The address of the remote server to proxy through.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_SERVER environment variable if set.")
	pingCmd.Flags().StringP("alias", "a", "default", "The server alias to use.")

	RootCmd.AddCommand(pingCmd)
}

var pingCmd = &cobra.Command{
	Use:   "ping",
	Short: "Ping the server",
	Long:  `Ping the server and display the health and version number.`,
	Args:  cobra.NoArgs,
	PreRun: func(cmd *cobra.Command, args []string) {
		alias, _ := cmd.Flags().GetString("alias")

		viper.BindPFlag("client."+alias+".server", cmd.Flags().Lookup("server"))
		viper.BindEnv("client."+alias+".server", config.CONFIG_ENV_PREFIX+"_SERVER")
	},
	Run: func(cmd *cobra.Command, args []string) {
		alias, _ := cmd.Flags().GetString("alias")
		cfg := config.GetServerAddr(alias)
		fmt.Println("Pinging server: ", cfg.HttpServer)

		client, err := apiclient.NewClient(cfg.HttpServer, cfg.ApiToken, viper.GetBool("tls_skip_verify"))
		if err != nil {
			fmt.Println("Failed to create API client:", err)
			os.Exit(1)
		}

		version, err := client.Ping(context.Background())
		if err != nil {
			fmt.Println("Failed to ping server")
			os.Exit(1)
		}

		fmt.Println("\nServer is healthy")
		fmt.Println("Version: ", version)
	},
}
