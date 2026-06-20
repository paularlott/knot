package methods

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/internal/agentlink"
)

var MethodsCmd = &cli.Command{
	Name:        "methods",
	Usage:       "Manage space methods",
	Description: "Register and unregister JSON-RPC methods for the current space.",
	Commands: []*cli.Command{
		registerCmd,
		unregisterCmd,
	},
}

var registerCmd = &cli.Command{
	Name:      "register",
	Usage:     "Register methods from a TOML or Scriptling file",
	MaxArgs:   cli.NoArgs,
	Arguments: []cli.Argument{&cli.StringArg{Name: "file", Usage: "A .toml method registration file or a .py Scriptling script that calls server.register()", Required: true}},
	Run: func(ctx context.Context, cmd *cli.Command) error {
		path := cmd.GetStringArg("file")
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}
		req := agentlink.RegisterMethodsFileRequest{Content: string(data)}

		var response agentlink.RegisterMethodsResponse
		switch ext := filepath.Ext(path); ext {
		case ".toml":
			// The agent daemon parses the TOML and forwards the registration
			// to the knot server via agentClient.RegisterMethods.
			if err := agentlink.SendWithResponseMsg(agentlink.CommandRegisterMethodsTOML, req, &response); err != nil {
				return err
			}
		case ".py":
			// The agent daemon runs the script in-process so server.register()
			// can publish directly through agentClient.RegisterMethods.
			if err := agentlink.SendWithResponseMsg(agentlink.CommandRegisterMethodsScript, req, &response); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported file extension %q (expected .toml or .py)", ext)
		}

		if !response.Success {
			return errors.New(response.Error)
		}
		fmt.Println("Methods registered")
		return nil
	},
}

var unregisterCmd = &cli.Command{
	Name:      "unregister",
	Usage:     "Remove all registered methods and stop the method server",
	MaxArgs:   cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		var response agentlink.RegisterMethodsResponse
		if err := agentlink.SendWithResponseMsg(agentlink.CommandUnregisterMethods, nil, &response); err != nil {
			return err
		}
		if !response.Success {
			return errors.New(response.Error)
		}
		fmt.Println("Methods unregistered")
		return nil
	},
}
