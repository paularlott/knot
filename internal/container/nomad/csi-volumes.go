package nomad

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/paularlott/knot/internal/database/model"
)

func (client *NomadClient) CreateCSIVolume(volume *model.CSIVolume) error {
	var volumes = model.CSIVolumes{}
	volumes.Volumes = append(volumes.Volumes, *volume)

	// If Id not set then use the name
	if volume.Id == "" {
		volume.Id = volume.Name
	}

	client.logger.Debug("creating csi volume", "volume_id", volume.Id)

	_, err := client.httpClient.Put(context.Background(), fmt.Sprintf("/v1/volume/csi/%s/create", volume.Id), &volumes, nil, http.StatusOK)
	if err != nil {
		client.logger.WithError(err).Debug("creating csi volume error", "volume_id", volume.Id)
		return err
	}

	return nil
}

func (client *NomadClient) DeleteCSIVolume(id string, namespace string) error {
	client.logger.Debug("deleting csi volume", "id", id)

	code, err := client.httpClient.Delete(context.Background(), fmt.Sprintf("/v1/volume/csi/%s/delete?namespace=%s", id, namespace), nil, nil, http.StatusOK)
	if err != nil {
		// Ignore 500 errors where error includes "volume not found"
		if code != http.StatusInternalServerError || !strings.Contains(err.Error(), "volume not found") {
			client.logger.WithError(err).Debug("deleting csi volume error", "id", id)
			return err
		}
	}

	return nil
}
