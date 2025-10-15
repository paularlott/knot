package nomad

import (
	"fmt"

	"github.com/paularlott/knot/internal/database/model"

	"github.com/paularlott/knot/internal/log"
)

func (client *NomadClient) CreateVolume(vol *model.Volume, variables map[string]interface{}) error {

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
	log.Debug("nomad: creating volume:", "volume_id", volumes.Volumes[0].Id)

	// Create the volume
	switch volumes.Volumes[0].Type {
	case "csi":
		err = client.CreateCSIVolume(&volumes.Volumes[0])
	case "host":
		_, err = client.CreateHostVolume(&volumes.Volumes[0])
	default:
		err = fmt.Errorf("unsupported volume type: %s", volumes.Volumes[0].Type)
	}
	if err != nil {
		return err
	}

	log.Debug("nomad: volumes created")

	return nil
}

func (client *NomadClient) DeleteVolume(vol *model.Volume, variables map[string]interface{}) error {

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
	log.Debug("nomad: deleting volume:", "volume", volumes.Volumes[0].Name)

	switch volumes.Volumes[0].Type {
	case "csi":
		if volumes.Volumes[0].Id == "" {
			volumes.Volumes[0].Id = volumes.Volumes[0].Name
		}
		err = client.DeleteCSIVolume(volumes.Volumes[0].Id, volumes.Volumes[0].Namespace)
	case "host":
		var id string
		id, err = client.GetIdHostVolume(volumes.Volumes[0].Name, volumes.Volumes[0].Namespace)
		if err != nil {
			return err
		}
		err = client.DeleteHostVolume(id, volumes.Volumes[0].Namespace)
	default:
		err = fmt.Errorf("unsupported volume type: %s", volumes.Volumes[0].Type)
	}
	if err != nil {
		return err
	}

	log.Debug("nomad: volume deleted")

	return nil
}
