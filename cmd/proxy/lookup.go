package cmd_proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

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

    http.DefaultClient.Timeout = 10 * time.Second
    resp, err := http.Get(fmt.Sprintf("%s/lookup/%s", proxyCmdCfg.server, service))
    if err != nil || resp.StatusCode != http.StatusOK {
      fmt.Println("Failed to lookup service")
      os.Exit(1)
    }
    defer resp.Body.Close()

    lookup := web.LookupResponse{}
    err = json.NewDecoder(resp.Body).Decode(&lookup)
    if err != nil || lookup.Status != true {
      fmt.Println("Failed to parse response")
      os.Exit(1)
    }

    fmt.Println("\nservice: ", service)
    fmt.Println("   host: ", lookup.Host)
    fmt.Println("   port: ", lookup.Port)
  },
}
