package command_proxy

import (
	"fmt"
	"os"

	"github.com/paularlott/knot/api/apiv1"
	"github.com/paularlott/knot/command"
	"github.com/paularlott/knot/util/rest"

	"github.com/spf13/cobra"
)

var lookupCmd = &cobra.Command{
  Use:   "lookup <service> [flags]",
  Short: "Look up the IP & port of a service",
  Long:  `Looks up the IP & port of a service via a DNS SRV lookup against the service name.

The request is passed to the proxy server to be processed rather than run against the local resolver.`,
  Args: cobra.ExactArgs(1),
  Run: func(cmd *cobra.Command, args []string) {
    service := args[0]
    cfg := command.GetServerAddr()
    client := rest.NewClient(cfg.HttpServer, cfg.ApiToken)

    lookup, _, err := apiv1.CallLookup(client, service)
    if err != nil || !lookup.Status {
      fmt.Println("Failed to parse response")
      os.Exit(1)
    }

    fmt.Println("\nservice: ", service)
    fmt.Println("   host: ", lookup.Host)
    fmt.Println("   port: ", lookup.Port)
  },
}
