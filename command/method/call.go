package command_method

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/command/cmdutil"
	"github.com/paularlott/knot/internal/methods"
)

var callCmd = &cli.Command{
	Name:  "call",
	Usage: "Call a method",
	Arguments: []cli.Argument{
		&cli.StringArg{Name: "method", Usage: "The method name", Required: true},
		&cli.StringArg{Name: "params", Usage: "JSON params object"},
	},
	Run: func(ctx context.Context, cmd *cli.Command) error {
		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return err
		}

		params := cmd.GetStringArg("params")
		if params == "" {
			stat, _ := os.Stdin.Stat()
			if stat != nil && (stat.Mode()&os.ModeCharDevice) == 0 {
				data, err := io.ReadAll(os.Stdin)
				if err != nil {
					return err
				}
				params = string(data)
			}
		}
		if params == "" {
			params = "{}"
		}
		var raw json.RawMessage
		if err := json.Unmarshal([]byte(params), &raw); err != nil {
			return fmt.Errorf("invalid params JSON: %w", err)
		}

		response, err := client.CallMethod(ctx, &methods.JSONRPCRequest{
			JSONRPC: "2.0",
			Method:  cmd.GetStringArg("method"),
			Params:  raw,
			ID:      1,
		})
		if err != nil {
			return err
		}
		data, err := json.Marshal(response)
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	},
}
