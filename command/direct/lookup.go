package commands_direct

import (
	"fmt"

	"github.com/paularlott/knot/command"
	"github.com/paularlott/knot/util"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
  lookupCmd.Flags().StringP("nameserver", "n", "", "The nameserver to use for SRV lookups (default use system resolver).\nOverrides the " + command.CONFIG_ENV_PREFIX + "_NAMESERVER environment variable if set.")
  command.RootCmd.AddCommand(lookupCmd)
}

var lookupCmd = &cobra.Command{
  Use:   "lookup <service> [flags]",
  Short: "Look up the IP & port of a service",
  Long:  `Looks up the IP & port of a service via a DNS SRV lookup against the service name.`,
  Args: cobra.ExactArgs(1),
  PreRun: func(cmd *cobra.Command, args []string) {
    viper.BindPFlag("nameserver", cmd.Flags().Lookup("nameserver"))
    viper.BindEnv("nameserver", command.CONFIG_ENV_PREFIX + "_NAMESERVER")
    viper.SetDefault("nameserver", "")
  },
  Run: func(cmd *cobra.Command, args []string) {
    var host string
    var port string
    var err error

    service := args[0]

    host, port, err = util.GetTargetFromSRV(service, viper.GetString("nameserver"))
    if err != nil {
      host, err = util.GetIP(service, viper.GetString("nameserver"))
      if err != nil {
        fmt.Printf("Failed to find service %s\n", service)
      }
    }

    fmt.Println("\nservice: ", service)
    fmt.Println("   host: ", host)
    fmt.Println("   port: ", port)
  },
}
