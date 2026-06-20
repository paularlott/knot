package command_method

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command/cmdutil"
	"github.com/paularlott/knot/internal/methods"
)

var callCmd = &cli.Command{
	Name:  "call",
	Usage: "Call a method",
	Arguments: []cli.Argument{
		&cli.StringArg{Name: "method", Usage: "The method name", Required: true},
		&cli.StringArg{Name: "params", Usage: "JSON params object, or array of params objects when --batch is used"},
	},
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "batch",
			Usage: "Treat params as an array of JSON objects; each element is sent as a separate call to the same method",
		},
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

		methodName := cmd.GetStringArg("method")

		if cmd.GetBool("batch") {
			return callBatch(ctx, client, methodName, params)
		}

		return callSingle(ctx, client, methodName, params)
	},
}

func callSingle(ctx context.Context, client *apiclient.ApiClient, methodName, params string) error {
	if params == "" {
		params = "{}"
	}
	var raw json.RawMessage
	if err := json.Unmarshal([]byte(params), &raw); err != nil {
		return fmt.Errorf("invalid params JSON: %w", err)
	}

	response, err := client.CallMethod(ctx, &methods.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  methodName,
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
}

func callBatch(ctx context.Context, client *apiclient.ApiClient, methodName, params string) error {
	if params == "" {
		return fmt.Errorf("--batch requires a JSON array of params objects")
	}

	// Parse the params as an array of JSON objects.
	var paramObjects []json.RawMessage
	if err := json.Unmarshal([]byte(params), &paramObjects); err != nil {
		return fmt.Errorf("--batch params must be a JSON array of objects: %w", err)
	}
	if len(paramObjects) == 0 {
		return fmt.Errorf("--batch params array is empty")
	}

	// Build one JSON-RPC request per params object, all targeting the same method.
	items := make([]methods.JSONRPCRequest, len(paramObjects))
	for i, p := range paramObjects {
		items[i] = methods.JSONRPCRequest{
			JSONRPC: "2.0",
			Method:  methodName,
			Params:  p,
			ID:      i + 1,
		}
	}

	responses, err := client.CallMethodBatch(ctx, items)
	if err != nil {
		return err
	}

	data, err := json.Marshal(responses)
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
