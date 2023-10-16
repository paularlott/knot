package cmd_proxy

import (
	"strings"

	"github.com/paularlott/knot/cmd"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type cmdProxyFlags struct {
  server string
  wsServer string
}

func init() {
  proxyCmd.PersistentFlags().StringP("server", "s", "", "The address of the remote server to proxy through.\nOverrides the " + cmd.CONFIG_ENV_PREFIX + "_SERVER environment variable if set.")

  cmd.RootCmd.AddCommand(proxyCmd)
  proxyCmd.AddCommand(sshCmd)
  proxyCmd.AddCommand(portCmd)
  proxyCmd.AddCommand(lookupCmd)
}

var proxyCmd = &cobra.Command{
  Use:   "proxy",
  Short: "Proxy a connection",
  Long:  "Proxy a connection from the local host to a remote destination via the proxy server.",
  PersistentPreRun: func(ccmd *cobra.Command, args []string) {
    viper.BindPFlag("client.server", ccmd.PersistentFlags().Lookup("server"))
    viper.BindEnv("client.server", cmd.CONFIG_ENV_PREFIX + "_SERVER")
  },
  Run: func(cmd *cobra.Command, args []string) {
    cmd.Help()
    return
  },
}

func getCmdProxyFlags() cmdProxyFlags {
  flags := cmdProxyFlags{}

  flags.server = viper.GetString("client.server")

  // If flags.server empty then throw and error
  if flags.server == "" {
    cobra.CheckErr("Missing proxy server address")
  }

  // Fix up the address to a websocket address
  flags.server = strings.TrimSuffix(flags.server, "/")
  if strings.HasPrefix(flags.server, "http") {
    flags.wsServer = "ws" + flags.server[4:]
  } else {
    flags.wsServer = "ws://" + flags.server
  }

  return flags
}