package cmd

import (
	"fmt"
	"io/fs"
	"strings"

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

    // Show our notices
    fmt.Println(legal.AppLicence)
    sep()
    fmt.Println(legal.AppNotice)
    sep()

    // Now all the dependencies
    fs.WalkDir(legal.LicenseFiles, ".", func(path string, d fs.DirEntry, err error) error {
      if err == nil {
        data, err := legal.LicenseFiles.ReadFile(path)
        if(err == nil) {
          // Remove the leading "licenses/"
          modName := path[9:]
          modName = modName[:len(modName)-len(modName[strings.LastIndex(modName, "-"):])]
          fmt.Println(modName + "\n")

          fmt.Println(string(data))
          sep()
        }
      }

      return nil
    })
  },
}

func sep() {
  fmt.Println("\n------------------------------------------------------------")
  fmt.Print("\n")
}
