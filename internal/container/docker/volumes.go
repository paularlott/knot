package docker

import (
	"context"
	"fmt"
	"strings"

	"github.com/paularlott/knot/internal/database/model"

	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"gopkg.in/yaml.v3"
)

func (c *DockerClient) CreateVolume(vol *model.Volume, variables map[string]interface{}) error {
	c.Logger.Debug("creating volume")

	// Parse the volume definition to fill out the knot variables
	volumes, err := model.ResolveVariables(vol.Definition, nil, nil, nil, variables)
	if err != nil {
		return err
	}

	var volInfo volInfo
	err = yaml.Unmarshal([]byte(volumes), &volInfo)
	if err != nil {
		return err
	}

	// If not exactly 1 volume then fail
	if len(volInfo.Volumes) != 1 {
		return fmt.Errorf("volume definition must contain exactly 1 volume")
	}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation(), client.WithHost(c.Host))
	if err != nil {
		return err
	}

	for volName, _ := range volInfo.Volumes {
		c.Logger.Debug("creating volume:", "volname", volName)

		_, err := cli.VolumeCreate(context.Background(), volume.CreateOptions{Name: volName})
		if err != nil {
			return err
		}
	}

	c.Logger.Debug("volume created")

	return nil
}

func (c *DockerClient) DeleteVolume(vol *model.Volume, variables map[string]interface{}) error {
	c.Logger.Debug("deleting volume")

	// Parse the volume definition to fill out the knot variables
	volumes, err := model.ResolveVariables(vol.Definition, nil, nil, nil, variables)
	if err != nil {
		return err
	}

	var volInfo volInfo
	err = yaml.Unmarshal([]byte(volumes), &volInfo)
	if err != nil {
		return err
	}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation(), client.WithHost(c.Host))
	if err != nil {
		return err
	}

	for volName, _ := range volInfo.Volumes {
		c.Logger.Debug("deleting volume:", "volname", volName)

		err := cli.VolumeRemove(context.Background(), volName, true)
		if err != nil && !strings.Contains(err.Error(), "No such volume") {
			return err
		}
	}

	c.Logger.Debug("volume deleted")

	return nil
}
