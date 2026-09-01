package command_spaces

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command/cmdutil"

	"github.com/paularlott/cli"
)

// JobsCmd is the `knot space jobs` group: manage the jobs defined on a space.
// The definitions are stored on the space and pushed to its agent, so they
// survive restarts and can be edited while the space is stopped. These
// commands also work from inside a space: cmdutil.GetClient picks up the
// agent's own credentials via the agentlink socket.
var JobsCmd = &cli.Command{
	Name:        "jobs",
	Usage:       "Manage a space's scheduled jobs",
	Description: `Manage the jobs defined on a space. Job definitions are stored on the space and pushed to the agent, so they survive restarts.`,
	MaxArgs:     cli.NoArgs,
	Commands: []*cli.Command{
		JobsListCmd,
		JobsRunCmd,
		JobsAddCmd,
		JobsUpdateCmd,
		JobsRemoveCmd,
		JobsEnableCmd,
		JobsDisableCmd,
	},
}

// jobsSpaceId resolves a space name (or ID) to its ID.
func jobsSpaceId(ctx context.Context, client *apiclient.ApiClient, spaceName string) (string, error) {
	spaces, _, err := client.GetSpaces(ctx, "", false)
	if err != nil {
		return "", fmt.Errorf("failed to get spaces: %w", err)
	}

	for _, s := range spaces.Spaces {
		if s.Name == spaceName {
			return s.Id, nil
		}
	}
	return "", fmt.Errorf("space '%s' not found", spaceName)
}

// jobsClient builds an API client for jobs commands.
func jobsClient(cmd *cli.Command) (*apiclient.ApiClient, error) {
	client, err := cmdutil.GetClient(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to create API client: %w", err)
	}
	return client, nil
}
