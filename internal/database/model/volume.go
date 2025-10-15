package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/log"
)

// Template object
type Volume struct {
	Id            string        `json:"volume_id" db:"volume_id,pk"`
	Name          string        `json:"name" db:"name"`
	Zone          string        `json:"zone" db:"zone"`
	Platform      string        `json:"platform" db:"platform"`
	Definition    string        `json:"definition" db:"definition"`
	Active        bool          `json:"active" db:"active"`
	IsDeleted     bool          `json:"is_deleted" db:"is_deleted"`
	CreatedUserId string        `json:"created_user_id" db:"created_user_id"`
	CreatedAt     time.Time     `json:"created_at" db:"created_at"`
	UpdatedUserId string        `json:"updated_user_id" db:"updated_user_id"`
	UpdatedAt     hlc.Timestamp `json:"updated_at" db:"updated_at"`
}

func NewVolume(name string, definition string, userId string, platform string) *Volume {
	id, err := uuid.NewV7()
	if err != nil {
		log.Fatal(err.Error())
	}

	volume := &Volume{
		Id:            id.String(),
		Name:          name,
		Definition:    definition,
		Active:        false,
		Platform:      platform,
		CreatedUserId: userId,
		CreatedAt:     time.Now().UTC(),
		UpdatedUserId: userId,
		UpdatedAt:     hlc.Now(),
		Zone:          "",
	}

	return volume
}

func (volume *Volume) GetVolume(variables map[string]interface{}) (*CSIVolumes, error) {
	return LoadVolumesFromYaml(volume.Definition, nil, nil, nil, variables)
}
