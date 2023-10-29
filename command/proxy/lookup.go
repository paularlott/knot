package command_proxy

import (
	"fmt"
	"os"

	api_v1 "github.com/paularlott/knot/api/v1"
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
    proxyCmdCfg := command.GetProxyFlags()

    client := rest.NewClient(proxyCmdCfg.Server)

    lookup := api_v1.LookupResponse{}

    err := client.Get(fmt.Sprintf("/api/v1/lookup/%s", service), &lookup)
    if err != nil || !lookup.Status {
      fmt.Println("Failed to parse response")
      os.Exit(1)
    }

    fmt.Println("\nservice: ", service)
    fmt.Println("   host: ", lookup.Host)
    fmt.Println("   port: ", lookup.Port)
  },
}
