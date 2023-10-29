package command

import (
	"github.com/paularlott/knot/legal"

	"github.com/spf13/cobra"
)

func init() {
  RootCmd.PersistentFlags().MarkHidden("config")

  RootCmd.AddCommand(legalCmd)
}

var legalCmd = &cobra.Command{
  Use:   "legal",
  Short: "Show legal information",
  Long:  `Output all the legal notices.`,
  Args: cobra.NoArgs,
  Run: func(cmd *cobra.Command, args []string) {
    legal.ShowLicenses()
  },
}
