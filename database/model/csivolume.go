package model

import (
	"encoding/json"

	"github.com/paularlott/knot/util"

	"gopkg.in/yaml.v3"
)

//
// Define the data structures for CSI volumes
//

type CSIVolumeMountOptions struct {
	FsType     string   `yaml:"fs_type" json:"FsType"`
	MountFlags []string `yaml:"mount_flags" json:"MountFlags"`
}

type CSIVolumeCapability struct {
	AccessMode     string `yaml:"access_mode" json:"AccessMode"`
	AttachmentMode string `yaml:"attachment_mode" json:"AttachmentMode"`
}

type CSIVolume struct {
	Id           string                `yaml:"id" json:"ID"`
	Name         string                `yaml:"name" json:"Name"`
	Namespace    string                `yaml:"namespace" json:"Namespace"`
	PuluginId    string                `yaml:"plugin_id" json:"PluginID"`
	Type         string                `yaml:"type" json:"Type"`
	MountOptions CSIVolumeMountOptions `yaml:"mount_options" json:"MountOptions"`
	CapacityMin  interface{}           `yaml:"capacity_min,omitempty" json:"RequestedCapacityMin,omitempty"`
	CapacityMax  interface{}           `yaml:"capacity_max,omitempty" json:"RequestedCapacityMax,omitempty"`
	Capabilities []CSIVolumeCapability `yaml:"capabilities,omitempty" json:"RequestedCapabilities,omitempty"`
	Secrets      map[string]string     `yaml:"secrets,omitempty" json:"Secrets,omitempty"`
	Parameters   map[string]string     `yaml:"parameters,omitempty" json:"Parameters,omitempty"`
}

type CSIVolumes struct {
	Volumes []CSIVolume `yaml:"volumes" json:"Volumes"`
}

func LoadVolumesFromYaml(yamlData string, t *Template, space *Space, user *User, variables *map[string]interface{}, applySpaceSizes bool) (*CSIVolumes, error) {
	var err error
	volumes := &CSIVolumes{}

	if space != nil || user != nil {
		yamlData, err = ResolveVariables(yamlData, t, space, user, variables)
		if err != nil {
			return nil, err
		}
	}

	err = yaml.Unmarshal([]byte(yamlData), volumes)
	if err != nil {
		return nil, err
	}

	// If applying sizes from space then resolve the variables
	var volumeSizes map[string]int64
	if applySpaceSizes {
		// Convert space volume sizes to a string
		temp, err := json.Marshal(space.VolumeSizes)
		if err != nil {
			return nil, err
		}

		// Resolve the variables
		data, err := ResolveVariables(string(temp), t, space, user, variables)
		if err != nil {
			return nil, err
		}

		// Convert back to a map
		err = json.Unmarshal([]byte(data), &volumeSizes)
		if err != nil {
			return nil, err
		}
	}

	// Fix up the capacity values & namespace
	for i, _ := range volumes.Volumes {
		if volumes.Volumes[i].CapacityMin != nil {
			switch volumes.Volumes[i].CapacityMin.(type) {
			case string:
				value, err := util.ConvertToBytes(volumes.Volumes[i].CapacityMin.(string))
				if err != nil {
					return nil, err
				}
				volumes.Volumes[i].CapacityMin = value
			}
		} else {
			volumes.Volumes[i].CapacityMin = nil
		}

		if volumes.Volumes[i].CapacityMax != nil {
			switch volumes.Volumes[i].CapacityMax.(type) {
			case string:
				value, err := util.ConvertToBytes(volumes.Volumes[i].CapacityMax.(string))
				if err != nil {
					return nil, err
				}
				volumes.Volumes[i].CapacityMax = value
			}
		} else {
			volumes.Volumes[i].CapacityMax = nil
		}

		// Apply space sizes if requested
		if applySpaceSizes {
			if volumeSizes[volumes.Volumes[i].Id] > 0 {
				volumes.Volumes[i].CapacityMin = volumeSizes[volumes.Volumes[i].Id] * 1024 * 1024 * 1024
				volumes.Volumes[i].CapacityMax = volumeSizes[volumes.Volumes[i].Id] * 1024 * 1024 * 1024
			}
		}

		// Fix namespace if nil then make default
		if volumes.Volumes[i].Namespace == "" {
			volumes.Volumes[i].Namespace = "default"
		}

		// default type to csi
		if volumes.Volumes[i].Type == "" {
			volumes.Volumes[i].Type = "csi"
		}

		// fix secrets and parameters
		if volumes.Volumes[i].Secrets == nil {
			volumes.Volumes[i].Secrets = make(map[string]string)
		}

		if volumes.Volumes[i].Parameters == nil {
			volumes.Volumes[i].Parameters = make(map[string]string)
		}
	}

	return volumes, nil
}
