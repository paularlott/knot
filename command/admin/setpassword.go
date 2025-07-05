package commands_admin

import (
	"context"
	"fmt"
	"syscall"

	"github.com/paularlott/knot/internal/database"

	"github.com/paularlott/cli"
	"github.com/paularlott/gossip/hlc"
	"golang.org/x/term"
)

var SetPasswordCmd = &cli.Command{
	Name:        "set-password",
	Usage:       "Reset a users password",
	Description: "Set a new password for the specified user.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "email-address",
			Usage:    "The email address of the user to reset the password for",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		email := cmd.GetStringArg("email-address")

		// Display what is going to happen and warning
		fmt.Println("Setting new password for user: ", email)

		// Prompt the user to enter the new password
		fmt.Printf("Enter the new password: ")
		password, err := term.ReadPassword(int(syscall.Stdin))
		if err != nil {
			return fmt.Errorf("Failed to read password: %w", err)
		}
		fmt.Println()

		// Connect to the database
		db := database.GetInstance()

		// Load the user
		user, err := db.GetUserByEmail(email)
		if err != nil {
			return fmt.Errorf("Error getting user: %w", err)
		}

		// Set the new password
		user.SetPassword(string(password))
		user.UpdatedAt = hlc.Now()

		// Save the user
		err = db.SaveUser(user, []string{"Password", "UpdatedAt"})
		if err != nil {
			return fmt.Errorf("Error saving user: %w", err)
		}

		fmt.Print("\nPassword set\n")
		return nil
	},
}
