package commands_admin

import (
	"fmt"

	"github.com/paularlott/knot/database"

	"github.com/spf13/cobra"
)

var resetTOTPCmd = &cobra.Command{
	Use:   "reset-totp <email address> [flags]",
	Short: "Reset the TOTP for the user",
	Long:  `Clear the current TOTP for the specified user so that on next login a new secret is generated.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {

		// Display what is going to happen and warning
		fmt.Println("Resetting TOTP for user: ", args[0])

		// Connect to the database
		db := database.GetInstance()

		// Load the user
		user, err := db.GetUserByEmail(args[0])
		if err != nil {
			fmt.Println("Error getting user: ", err)
			return
		}

		// Clear TOTP
		user.TOTPSecret = ""

		// Save the user
		err = db.SaveUser(user)
		if err != nil {
			fmt.Println("Error saving user: ", err)
			return
		}

		fmt.Print("\nTOTP Reset\n")
	},
}
