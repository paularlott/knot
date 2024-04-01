package nomad

import (
	"fmt"

	"github.com/paularlott/knot/database/model"

	"github.com/rs/zerolog/log"
)

func (client *NomadClient) CreateVolume(vol *model.Volume, variables *map[string]interface{}) error {

	// Get the volume definitions
	volumes, err := vol.GetVolume(variables)
	if err != nil {
		return err
	}

	// If not exactly 1 volume then fail
	if len(volumes.Volumes) != 1 {
		return fmt.Errorf("volume definition must contain exactly 1 volume")
	}

	// Display the name of the volume
	log.Debug().Msgf("nomad: creating volume: %s", volumes.Volumes[0].Id)

	// Create the volume
	err = client.CreateCSIVolume(&volumes.Volumes[0])
	if err != nil {
		return err
	}

	log.Debug().Msg("nomad: volumes created")

	return nil
}

func (client *NomadClient) DeleteVolume(vol *model.Volume, variables *map[string]interface{}) error {

	// Get the volume definitions
	volumes, err := vol.GetVolume(variables)
	if err != nil {
		return err
	}

	// If not exactly 1 volume then fail
	if len(volumes.Volumes) != 1 {
		return fmt.Errorf("volume definition must contain exactly 1 volume")
	}

	// Display the name of the volume
	log.Debug().Msgf("nomad: deleting volume: %s", volumes.Volumes[0].Id)

	err = client.DeleteCSIVolume(volumes.Volumes[0].Id, volumes.Volumes[0].Namespace)
	if err != nil {
		return err
	}

	log.Debug().Msg("nomad: volume deleted")

	return nil
}
