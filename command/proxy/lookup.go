package command_proxy

import (
	"fmt"
	"os"

	"github.com/paularlott/knot/internal/api"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/util/rest"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var lookupCmd = &cobra.Command{
	Use:   "lookup <service> [flags]",
	Short: "Look up the IP & port of a service",
	Long: `Looks up the IP & port of a service via a DNS SRV lookup against the service name.

The request is passed to the proxy server to be processed rather than run against the local resolver.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		service := args[0]
		alias, _ := cmd.Flags().GetString("alias")
		cfg := config.GetServerAddr(alias)
		client, err := rest.NewClient(cfg.HttpServer, cfg.ApiToken, viper.GetBool("tls_skip_verify"))
		if err != nil {
			fmt.Println("Failed to create API client:", err)
			os.Exit(1)
		}

		lookup, _, err := api.CallLookup(client, service)
		if err != nil || !lookup.Status {
			fmt.Println("Failed to parse response")
			os.Exit(1)
		}

		fmt.Println("\nservice: ", service)
		fmt.Println("   host: ", lookup.Host)
		fmt.Println("   port: ", lookup.Port)
	},
}
