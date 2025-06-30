package commands_admin

import (
	"context"
	"fmt"
	"time"

	"github.com/paularlott/knot/internal/database"

	"github.com/paularlott/cli"
)

var ResetTOTPCmd = &cli.Command{
	Name:        "reset-totp",
	Usage:       "Reset a users TOTP",
	Description: "Clear the current TOTP for the specified user so that on next login a new secret is generated.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "email-address",
			Usage:    "The email address of the user to reset the TOTP for",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		email := cmd.GetStringArg("email-address")

		// Display what is going to happen and warning
		fmt.Println("Resetting TOTP for user: ", email)

		// Connect to the database
		db := database.GetInstance()

		// Load the user
		user, err := db.GetUserByEmail(email)
		if err != nil {
			fmt.Println("Error getting user: ", err)
			return nil
		}

		// Clear TOTP
		user.TOTPSecret = ""
		user.UpdatedAt = time.Now().UTC()

		// Save the user
		err = db.SaveUser(user, []string{"TOTPSecret", "UpdatedAt"})
		if err != nil {
			fmt.Println("Error saving user: ", err)
			return nil
		}

		fmt.Print("\nTOTP Reset\n")
		return nil
	},
}
