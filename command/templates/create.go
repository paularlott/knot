package command_templates

import (
	"fmt"
	"os"

	"github.com/paularlott/knot/apiclient"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	createCmd.Flags().StringP("description", "d", "", "Description of the template.")
	createCmd.Flags().StringP("job", "j", "", "The file to load for the nomad job description.")
	createCmd.Flags().StringP("volume", "v", "", "The YAML file to load for the volume description.")
	createCmd.Flags().StringSliceP("group", "g", []string{}, "Define a group to limit the template visibility to, can be given multiple times.")
	createCmd.Flags().Bool("local-container", false, "Create a local container template.")
	createCmd.Flags().Bool("with-terminal", false, "Enable terminal for the template.")
	createCmd.Flags().Bool("with-vscode-tunnel", false, "Enable VSCode tunnel for the template.")
	createCmd.Flags().Bool("with-code-server", false, "Enable Code Server for the template.")
}

var createCmd = &cobra.Command{
	Use:   "create <name> [flags]",
	Short: "Create a template",
	Long:  `Create a new template.`,
	Args:  cobra.ExactArgs(1),
	PreRun: func(cmd *cobra.Command, args []string) {
		viper.BindPFlag("description", cmd.Flags().Lookup("description"))
		viper.SetDefault("description", "")

		viper.BindPFlag("job", cmd.Flags().Lookup("job"))
		viper.SetDefault("job", "")

		viper.BindPFlag("volume", cmd.Flags().Lookup("volume"))
		viper.SetDefault("volume", "")

		viper.BindPFlag("group", cmd.Flags().Lookup("group"))
		viper.SetDefault("group", []string{})

		viper.BindPFlag("local-container", cmd.Flags().Lookup("local-container"))
		viper.SetDefault("local-container", false)

		viper.BindPFlag("with-terminal", cmd.Flags().Lookup("with-terminal"))
		viper.SetDefault("with-terminal", false)

		viper.BindPFlag("with-vscode-tunnel", cmd.Flags().Lookup("with-vscode-tunnel"))
		viper.SetDefault("with-vscode-tunnel", false)

		viper.BindPFlag("with-code-server", cmd.Flags().Lookup("with-code-server"))
		viper.SetDefault("with-code-server", false)
	},
	Run: func(cmd *cobra.Command, args []string) {

		// Check job given
		if viper.GetString("job") == "" {
			fmt.Println("Job file not given.")
			return
		}

		// Load the job file into a string
		job, err := loadFile(viper.GetString("job"))
		if err != nil {
			fmt.Println("Error loading job file: ", err)
			return
		}

		// Load the volume file into a string if given
		volume := ""
		if viper.GetString("volume") != "" {
			volume, err = loadFile(viper.GetString("volume"))
			if err != nil {
				fmt.Println("Error loading volume file: ", err)
				return
			}
		}

		client := apiclient.NewClient(viper.GetString("client.server"), viper.GetString("client.token"), viper.GetBool("tls_skip_verify"))

		// Get the available groups
		groups, _, err := client.GetGroups()
		if err != nil {
			fmt.Println("Error getting groups: ", err)
			return
		}

		// Convert group names to IDs
		groupIds := []string{}
		for _, group := range groups.Groups {
			for _, name := range viper.GetStringSlice("group") {
				if group.Name == name {
					groupIds = append(groupIds, group.Id)
					break
				}
			}
		}

		// Create the template
		_, _, err = client.CreateTemplate(
			args[0],
			job,
			viper.GetString("description"),
			volume,
			groupIds,
			viper.GetBool("local-container"),
			false,
			viper.GetBool("with-terminal"),
			viper.GetBool("with-vscode-tunnel"),
			viper.GetBool("with-code-server"),
		)
		if err != nil {
			fmt.Println("Error creating template: ", err)
			return
		}

		fmt.Println("Template created: ", args[0])
	},
}

func loadFile(filename string) (string, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
