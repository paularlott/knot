package command

import (
	"fmt"
	"os"

	api_v1 "github.com/paularlott/knot/api/v1"
	"github.com/paularlott/knot/util/rest"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
  pingCmd.Flags().StringP("server", "s", "", "The address of the remote server to proxy through.\nOverrides the " + CONFIG_ENV_PREFIX + "_SERVER environment variable if set.")

  RootCmd.AddCommand(pingCmd)
}

var pingCmd = &cobra.Command{
  Use:   "ping",
  Short: "Ping the server",
  Long:  `Ping the server and display the health and version number.`,
  Args: cobra.NoArgs,
  PreRun: func(cmd *cobra.Command, args []string) {
    viper.BindPFlag("client.server", cmd.Flags().Lookup("server"))
    viper.BindEnv("client.server", CONFIG_ENV_PREFIX + "_SERVER")
  },
  Run: func(cmd *cobra.Command, args []string) {
    server := viper.GetString("client.server")

    fmt.Println("Pinging server: ", server)

    client := rest.NewClient(server)

    ping := api_v1.PingResponse{}

    err := client.Get("/api/v1/ping", &ping)
    if err != nil || !ping.Status {
      fmt.Println("Failed to ping server")
      os.Exit(1)
    }

    fmt.Println("\nServer is healthy")
    fmt.Println("Version: ", ping.Version)
  },
}
