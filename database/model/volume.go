package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// Template object
type Volume struct {
	Id            string    `json:"volume_id"`
	Name          string    `json:"name"`
	Location      string    `json:"location"`
	Definition    string    `json:"definition"`
	Active        bool      `json:"active"`
	CreatedUserId string    `json:"created_user_id"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedUserId string    `json:"updated_user_id"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func NewVolume(name string, definition string, userId string) *Volume {
	id, err := uuid.NewV7()
	if err != nil {
		log.Fatal().Msg(err.Error())
	}

	volume := &Volume{
		Id:            id.String(),
		Name:          name,
		Definition:    definition,
		Active:        false,
		CreatedUserId: userId,
		CreatedAt:     time.Now().UTC(),
		UpdatedUserId: userId,
		UpdatedAt:     time.Now().UTC(),
		Location:      "",
	}

	return volume
}

func (volume *Volume) GetVolume(variables *map[string]interface{}) (*CSIVolumes, error) {
	return LoadVolumesFromYaml(volume.Definition, nil, nil, nil, variables, false)
}
