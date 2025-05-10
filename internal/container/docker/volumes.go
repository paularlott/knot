package docker

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/database/model"
	"github.com/spf13/viper"

	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

func (c *DockerClient) CreateVolume(vol *model.Volume, variables *map[string]interface{}) error {
	log.Debug().Msg("docker: creating volume")

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

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation(), client.WithHost(viper.GetString("server.docker.host")))
	if err != nil {
		return err
	}

	for volName, _ := range volInfo.Volumes {
		log.Debug().Msgf("docker: creating volume: %s", volName)

		_, err := cli.VolumeCreate(context.Background(), volume.CreateOptions{Name: volName})
		if err != nil {
			return err
		}
	}

	log.Debug().Msg("docker: volume created")

	return nil
}

func (c *DockerClient) DeleteVolume(vol *model.Volume, variables *map[string]interface{}) error {
	log.Debug().Msg("docker: deleting volume")

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

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation(), client.WithHost(viper.GetString("server.docker.host")))
	if err != nil {
		return err
	}

	for volName, _ := range volInfo.Volumes {
		log.Debug().Msgf("docker: deleting volume: %s", volName)

		err := cli.VolumeRemove(context.Background(), volName, true)
		if err != nil {
			return err
		}
	}

	log.Debug().Msg("docker: volume deleted")

	return nil
}
