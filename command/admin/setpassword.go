package commands_admin

import (
	"fmt"
	"os"
	"syscall"

	"github.com/paularlott/knot/database"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var setPasswordCmd = &cobra.Command{
	Use:   "set-password <email address> [flags]",
	Short: "Set a password for the user",
	Long:  `Set a new password for the specified user.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {

		// Display what is going to happen and warning
		fmt.Println("Setting new password for user: ", args[0])

		// Prompt the user to enter the new password
		fmt.Printf("Enter the new password: ")
		password, err := term.ReadPassword(int(syscall.Stdin))
		if err != nil {
			fmt.Println("Failed to read password")
			os.Exit(1)
		}
		fmt.Println()

		// Connect to the database
		db := database.GetInstance()

		// Load the user
		user, err := db.GetUserByEmail(args[0])
		if err != nil {
			fmt.Println("Error getting user: ", err)
			return
		}

		// Set the new password
		user.SetPassword(string(password))

		// Save the user
		err = db.SaveUser(user)
		if err != nil {
			fmt.Println("Error saving user: ", err)
			return
		}

		fmt.Print("\nPassword set\n")
	},
}
