package nomad

import (
	"context"
	"fmt"
	"net/http"

	"github.com/paularlott/knot/internal/database/model"

	"github.com/rs/zerolog/log"
)

type hostVolCreateResponse struct {
	ID string `json:"ID"`
}

type hostCreateResponse struct {
	Volume hostVolCreateResponse `json:"Volume"`
}

func (client *NomadClient) CreateHostVolume(volume *model.CSIVolume) (string, error) {
	log.Debug().Msgf("nomad: creating host volume %s", volume.Id)

	// Convert CSI volume to Nomad host volume structure
	hostVolume := map[string]interface{}{
		"Name":      volume.Name,
		"Namespace": volume.Namespace,
		"Type":      "host",
		"PluginID":  volume.PuluginId,
	}

	// Add capacity constraints if specified
	if volume.CapacityMin != nil {
		hostVolume["RequestedCapacityMin"] = volume.CapacityMin
	}
	if volume.CapacityMax != nil {
		hostVolume["RequestedCapacityMax"] = volume.CapacityMax
	}

	// Add capabilities if specified
	if len(volume.Capabilities) > 0 {
		hostVolume["RequestedCapabilities"] = volume.Capabilities
	}

	// Add secrets if specified
	if len(volume.Secrets) > 0 {
		hostVolume["Secrets"] = volume.Secrets
	}

	// Add parameters if specified
	if len(volume.Parameters) > 0 {
		hostVolume["Parameters"] = volume.Parameters
	}

	// Add mount options if specified
	if volume.MountOptions.FsType != "" || len(volume.MountOptions.MountFlags) > 0 {
		mountOptions := map[string]interface{}{}
		if volume.MountOptions.FsType != "" {
			mountOptions["FsType"] = volume.MountOptions.FsType
		}
		if len(volume.MountOptions.MountFlags) > 0 {
			mountOptions["MountFlags"] = volume.MountOptions.MountFlags
		}
		hostVolume["MountOptions"] = mountOptions
	}

	vol := map[string]interface{}{
		"Volume": hostVolume,
	}

	var response hostCreateResponse
	_, err := client.httpClient.Put(context.Background(), "/v1/volume/host/create", &vol, &response, http.StatusOK)
	if err != nil {
		log.Debug().Msgf("nomad: creating host volume %s, error: %s", volume.Id, err)
		return "", err
	}

	return response.Volume.ID, nil
}

func (client *NomadClient) DeleteHostVolume(id string, namespace string) error {
	log.Debug().Msgf("nomad: deleting host volume %s", id)
	code, err := client.httpClient.Delete(context.Background(), fmt.Sprintf("/v1/volume/host/%s/delete?namespace=%s", id, namespace), nil, nil, http.StatusOK)
	if err != nil && code != http.StatusNotFound {
		log.Debug().Msgf("nomad: deleting csi volume %s, error: %v, code: %d", id, err, code)
		return err
	}

	return nil
}

type volumeListVolume struct {
	Name string `json:"Name"`
	ID   string `json:"ID"`
}

func (client *NomadClient) GetIdHostVolume(name, namespace string) (string, error) {
	var response []volumeListVolume
	_, err := client.httpClient.Get(context.Background(), fmt.Sprintf("/v1/volumes?type=host&namespace=%s", namespace), &response)
	if err != nil {
		return "", err
	}

	for _, volume := range response {
		if volume.Name == name {
			return volume.ID, nil
		}
	}

	return "", fmt.Errorf("host volume not found")
}
