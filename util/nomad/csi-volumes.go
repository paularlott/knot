package nomad

import (
	"fmt"
	"net/http"

	"github.com/paularlott/knot/database/model"

	"github.com/rs/zerolog/log"
)

func (client *NomadClient) CreateCSIVolume(volume *model.Volume) error {
  var volumes = model.Volumes{}
  volumes.Volumes = append(volumes.Volumes, *volume)

  log.Debug().Msgf("nomad: creating csi volume %s", volume.Id)

  _, err := client.httpClient.Put(fmt.Sprintf("/v1/volume/csi/%s/create", volume.Id), &volumes, nil, http.StatusOK)
  if err != nil {
    log.Debug().Msgf("nomad: creating csi volume %s, error: %s", volume.Id, err)
    return err
  }

  return nil
}

func (client *NomadClient) DeleteCSIVolume(volume *model.SpaceVolume) error {
  log.Debug().Msgf("nomad: deleting csi volume %s", volume.Id)

  _, err := client.httpClient.Delete(fmt.Sprintf("/v1/volume/csi/%s/delete?namespace=%s", volume.Id, volume.Namespace), nil, nil, http.StatusOK)
  if err != nil {
    log.Debug().Msgf("nomad: deleting csi volume %s, error: %s", volume.Id, err)
    return err
  }

  return nil
}
