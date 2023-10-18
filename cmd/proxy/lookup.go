package cmd_proxy

import (
	"fmt"
	"os"

	"github.com/paularlott/knot/util/rest"
	"github.com/paularlott/knot/web"

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
    proxyCmdCfg := getCmdProxyFlags()

    client := rest.NewClient(proxyCmdCfg.server)

    lookup := web.LookupResponse{}

    err := client.Get(fmt.Sprintf("/lookup/%s", service), &lookup)
    if err != nil || lookup.Status != true {
      fmt.Println("Failed to parse response")
      os.Exit(1)
    }

    fmt.Println("\nservice: ", service)
    fmt.Println("   host: ", lookup.Host)
    fmt.Println("   port: ", lookup.Port)
  },
}
