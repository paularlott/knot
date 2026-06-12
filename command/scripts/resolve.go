package command_scripts

import (
	"context"
	"fmt"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
)

func resolveScript(ctx context.Context, cmd *cli.Command, client *apiclient.ApiClient, name string) (*apiclient.ScriptDetails, error) {
	global := cmd.GetBool("global")

	script, err := client.GetScriptDetailsByName(ctx, name)
	if err != nil || script.Id == "" {
		return nil, fmt.Errorf("script %s not found", name)
	}

	if global && script.UserId != "" {
		scripts, err := client.GetScripts(ctx)
		if err != nil {
			return nil, fmt.Errorf("error listing scripts: %w", err)
		}
		for _, s := range scripts.Scripts {
			if s.Name == name && s.UserId == "" {
				return client.GetScript(ctx, s.Id)
			}
		}
		return nil, fmt.Errorf("global script %s not found", name)
	}

	return script, nil
}
