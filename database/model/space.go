package model

import (
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

type SpaceVolume struct {
	Id        string `json:"id"`
	Namespace string `json:"Namespace"`
}

// Space object
type Space struct {
	Id             string                 `json:"space_id"`
	UserId         string                 `json:"user_id"`
	TemplateId     string                 `json:"template_id"`
	Name           string                 `json:"name"`
	Shell          string                 `json:"shell"`
	Location       string                 `json:"location"`
	TemplateHash   string                 `json:"template_hash"`
	NomadNamespace string                 `json:"nomad_namespace"`
	ContainerId    string                 `json:"container_id"`
	VolumeData     map[string]SpaceVolume `json:"volume_data"`
	VolumeSizes    map[string]int64       `json:"volume_sizes"`
	IsDeployed     bool                   `json:"is_deployed"`
	IsPending      bool                   `json:"is_pending"` // Flags if the space is pending a state change, starting or stopping
	IsDeleting     bool                   `json:"is_deleting"`
	AltNames       []string               `json:"alt_names"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
}

func NewSpace(name string, userId string, templateId string, shell string, volSizes *map[string]int64, altNames *[]string) *Space {
	id, err := uuid.NewV7()
	if err != nil {
		log.Fatal().Msg(err.Error())
	}

	space := &Space{
		Id:           id.String(),
		UserId:       userId,
		TemplateId:   templateId,
		Name:         name,
		AltNames:     *altNames,
		Shell:        shell,
		TemplateHash: "",
		IsDeployed:   false,
		IsPending:    false,
		IsDeleting:   false,
		VolumeData:   make(map[string]SpaceVolume),
		VolumeSizes:  *volSizes,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
		Location:     "",
	}

	return space
}

// Get the storage size for the space in GB
func (space *Space) GetStorageSize(template *Template) (int, error) {
	var sizeGB int = 0

	if !template.LocalContainer {

		// Get the volumes with sizes applied
		volumes, err := template.GetVolumes(space, nil, nil, true)
		if err != nil {
			return 0, err
		}

		// Calculate the volume sizes
		for _, volume := range volumes.Volumes {
			if volume.CapacityMin != nil {
				sizeGB += int(math.Max(1, math.Ceil(float64(volume.CapacityMin.(int64))/(1024*1024*1024))))
			}
		}
	}

	return sizeGB, nil
}
