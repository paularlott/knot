package command

import (
	"fmt"

	"github.com/paularlott/knot/internal/util/crypt"

	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(genkeyCmd)
}

var genkeyCmd = &cobra.Command{
	Use:   "genkey",
	Short: "Generate Encryption Key",
	Long:  `Generate an encryption key for encrypting stored variables.`,
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		key := crypt.CreateKey()
		fmt.Println("Encryption Key:", key)
		fmt.Println("")
	},
}
