package command_spaces

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/database/model"
)

// jobsSaveDefinitions fetches the space's current job definitions, lets the
// caller mutate them, and PUTs the result back.
func jobsSaveDefinitions(ctx context.Context, client *apiclient.ApiClient, spaceId string, mutate func(jobs *[]model.SpaceJob) error) error {
	definitions, code, err := client.GetSpaceJobs(ctx, spaceId)
	if err != nil {
		if code == 404 {
			return fmt.Errorf("space not found")
		}
		return fmt.Errorf("failed to get jobs: %w", err)
	}

	if err := mutate(&definitions.Jobs); err != nil {
		return err
	}

	_, code, err = client.UpdateSpaceJobs(ctx, spaceId, &apiclient.SpaceJobsRequest{
		Jobs:    definitions.Jobs,
		Enabled: definitions.Enabled,
	})
	if err != nil {
		if code == 400 {
			return fmt.Errorf("invalid job definitions: %w", err)
		} else if code == 403 {
			return fmt.Errorf("no permission to update jobs")
		} else if code == 404 {
			return fmt.Errorf("space not found")
		}
		return fmt.Errorf("failed to update jobs: %w", err)
	}
	return nil
}
