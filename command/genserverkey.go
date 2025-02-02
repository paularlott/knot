package command

import (
	"fmt"

	"github.com/paularlott/knot/util/crypt"

	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(genServerKeyCmd)
}

var genServerKeyCmd = &cobra.Command{
	Use:   "gen-server-key",
	Short: "Generate a shared key for leaf and origin servers",
	Long:  `Generate a key for for use as the shared key between leaf and origin servers.`,
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		key, err := crypt.GenerateAPIKey()
		if err != nil {
			fmt.Println("Error generating key:", err)
			return
		}

		fmt.Println("Key:", key)
		fmt.Println("")
	},
}
