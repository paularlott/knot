package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// Group object
type Group struct {
	Id            string    `json:"group_id" db:"group_id,pk"`
	Name          string    `json:"name" db:"name"`
	MaxSpaces     uint32    `json:"max_spaces" db:"max_spaces"`
	ComputeUnits  uint32    `json:"compute_units" db:"compute_units"`
	StorageUnits  uint32    `json:"storage_units" db:"storage_units"`
	MaxTunnels    uint32    `json:"max_tunnels" db:"max_tunnels"`
	CreatedUserId string    `json:"created_user_id" db:"created_user_id"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
	UpdatedUserId string    `json:"updated_user_id" db:"updated_user_id"`
	UpdatedAt     time.Time `json:"updated_at" db:"updated_at"`
}

func NewGroup(name string, userId string, maxSpaces uint32, computeUnits uint32, storageUnits uint32, maxTunnels uint32) *Group {
	id, err := uuid.NewV7()
	if err != nil {
		log.Fatal().Msg(err.Error())
	}

	group := &Group{
		Id:            id.String(),
		Name:          name,
		MaxSpaces:     maxSpaces,
		ComputeUnits:  computeUnits,
		StorageUnits:  storageUnits,
		MaxTunnels:    maxTunnels,
		CreatedUserId: userId,
		CreatedAt:     time.Now().UTC(),
		UpdatedUserId: userId,
		UpdatedAt:     time.Now().UTC(),
	}

	return group
}
