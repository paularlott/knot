package nomad

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/paularlott/knot/internal/database/model"

	"github.com/rs/zerolog/log"
)

func (client *NomadClient) CreateCSIVolume(volume *model.CSIVolume) error {
	var volumes = model.CSIVolumes{}
	volumes.Volumes = append(volumes.Volumes, *volume)

	log.Debug().Msgf("nomad: creating csi volume %s", volume.Id)

	_, err := client.httpClient.Put(context.Background(), fmt.Sprintf("/v1/volume/csi/%s/create", volume.Id), &volumes, nil, http.StatusOK)
	if err != nil {
		log.Debug().Msgf("nomad: creating csi volume %s, error: %s", volume.Id, err)
		return err
	}

	return nil
}

func (client *NomadClient) DeleteCSIVolume(id string, namespace string) error {
	log.Debug().Msgf("nomad: deleting csi volume %s", id)

	code, err := client.httpClient.Delete(context.Background(), fmt.Sprintf("/v1/volume/csi/%s/delete?namespace=%s", id, namespace), nil, nil, http.StatusOK)
	if err != nil {
		// Ignore 500 errors where error includes "volume not found"
		if code != http.StatusInternalServerError || !strings.Contains(err.Error(), "volume not found") {
			log.Debug().Msgf("nomad: deleting csi volume %s, error: %s", id, err)
			return err
		}
	}

	return nil
}
