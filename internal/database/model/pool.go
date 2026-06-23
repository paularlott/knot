package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/log"
)

type PoolDefinition struct {
	Id              string        `json:"pool_id" db:"pool_id,pk" msgpack:"pool_id"`
	Name            string        `json:"name" db:"name" msgpack:"name"`
	TemplateId      string        `json:"template_id" db:"template_id" msgpack:"template_id"`
	StartupScriptId string        `json:"startup_script_id" db:"startup_script_id" msgpack:"startup_script_id"`
	DesiredCount    int           `json:"desired_count" db:"desired_count" msgpack:"desired_count"`
	Active          bool          `json:"active" db:"active" msgpack:"active"`
	IsDeleted       bool          `json:"is_deleted" db:"is_deleted" msgpack:"is_deleted"`
	CreatedUserId   string        `json:"created_user_id" db:"created_user_id" msgpack:"created_user_id"`
	CreatedAt       time.Time     `json:"created_at" db:"created_at" msgpack:"created_at"`
	UpdatedUserId   string        `json:"updated_user_id" db:"updated_user_id" msgpack:"updated_user_id"`
	UpdatedAt       hlc.Timestamp `json:"updated_at" db:"updated_at" msgpack:"updated_at"`
}

func NewPoolDefinition(name, templateId, startupScriptId string, desiredCount int, userId string) *PoolDefinition {
	id, err := uuid.NewV7()
	if err != nil {
		log.Fatal(err.Error())
	}

	now := time.Now().UTC()
	return &PoolDefinition{
		Id:              id.String(),
		Name:            name,
		TemplateId:      templateId,
		StartupScriptId: startupScriptId,
		DesiredCount:    desiredCount,
		Active:          true,
		CreatedUserId:   userId,
		CreatedAt:       now,
		UpdatedUserId:   userId,
		UpdatedAt:       hlc.Now(),
	}
}
