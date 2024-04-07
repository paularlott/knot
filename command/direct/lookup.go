package commands_direct

import (
	"fmt"

	"github.com/paularlott/knot/util"

	"github.com/spf13/cobra"
)

var lookupCmd = &cobra.Command{
	Use:   "lookup <service> [flags]",
	Short: "Look up the IP & port of a service",
	Long:  `Looks up the IP & port of a service via a DNS SRV lookup against the service name.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		service := args[0]

		hostPorts, err := util.LookupSRV(service)
		if err != nil {
			fmt.Println("Failed to find service")
			fmt.Println(err)
			return
		}

		fmt.Println("\nservice: ", service)
		for _, hp := range *hostPorts {
			fmt.Println("  ", hp.Host, hp.Port)
		}
	},
}
