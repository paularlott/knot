package docker

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/paularlott/knot/internal/database/model"
	"gopkg.in/yaml.v3"
)

type volumeCreateRequest struct {
	Name string `json:"Name"`
}

type volumeCreateResponse struct {
	Name string `json:"Name"`
}

func (c *DockerClient) volumeCreate(ctx context.Context, name string) (string, error) {
	var resp volumeCreateResponse
	code, err := c.httpClient.PostJSON(ctx, "/v1.41/volumes/create", volumeCreateRequest{Name: name}, &resp, http.StatusCreated)
	if err != nil {
		return "", fmt.Errorf("volume create failed (HTTP %d): %w", code, err)
	}
	return resp.Name, nil
}

func (c *DockerClient) volumeRemove(ctx context.Context, name string) error {
	// ?force=true matches original SDK behaviour (VolumeRemove called with force=true throughout).
	code, err := c.httpClient.Delete(ctx, "/v1.41/volumes/"+url.PathEscape(name)+"?force=true", nil, nil, http.StatusNoContent)
	if err != nil {
		if code == http.StatusNotFound {
			return nil
		}
		return fmt.Errorf("volume remove failed (HTTP %d): %w", code, err)
	}
	return nil
}

func (c *DockerClient) CreateVolume(vol *model.Volume, variables map[string]interface{}) error {
	c.Logger.Debug("creating volume")

	volumes, err := model.ResolveVariables(vol.Definition, nil, nil, nil, variables)
	if err != nil {
		return err
	}

	var vi volInfo
	if err = yaml.Unmarshal([]byte(volumes), &vi); err != nil {
		return err
	}

	if len(vi.Volumes) != 1 {
		return fmt.Errorf("volume definition must contain exactly 1 volume")
	}

	for volName := range vi.Volumes {
		c.Logger.Debug("creating volume", "name", volName)
		if _, err := c.volumeCreate(context.Background(), volName); err != nil {
			return err
		}
	}

	c.Logger.Debug("volume created")
	return nil
}

func (c *DockerClient) DeleteVolume(vol *model.Volume, variables map[string]interface{}) error {
	c.Logger.Debug("deleting volume")

	volumes, err := model.ResolveVariables(vol.Definition, nil, nil, nil, variables)
	if err != nil {
		return err
	}

	var vi volInfo
	if err = yaml.Unmarshal([]byte(volumes), &vi); err != nil {
		return err
	}

	for volName := range vi.Volumes {
		c.Logger.Debug("deleting volume", "name", volName)
		if err := c.volumeRemove(context.Background(), volName); err != nil {
			return err
		}
	}

	c.Logger.Debug("volume deleted")
	return nil
}
