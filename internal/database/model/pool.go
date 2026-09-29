package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/log"
)

type PoolDefinition struct {
	Id              string `json:"pool_id" db:"pool_id,pk" msgpack:"pool_id"`
	Name            string `json:"name" db:"name" msgpack:"name"`
	TemplateId      string `json:"template_id" db:"template_id" msgpack:"template_id"`
	StartupScriptId string `json:"startup_script_id" db:"startup_script_id" msgpack:"startup_script_id"`
	DesiredCount    int    `json:"desired_count" db:"desired_count" msgpack:"desired_count"`
	Active          bool   `json:"active" db:"active" msgpack:"active"`
	Zone            string `json:"zone" db:"zone" msgpack:"zone"`
	IsDeleted       bool   `json:"is_deleted" db:"is_deleted" msgpack:"is_deleted"`
	// Exclusive member leases. LeaseMaxTime is in seconds: 0 = leases
	// disabled (the default), -1 = no timeout (leases run until released),
	// >0 = maximum duration of one acquire/extend. LeaseMaxExtensions:
	// 0 = extending forbidden, -1 = unlimited, >0 = max extensions per lease.
	LeaseMaxTime       int           `json:"lease_max_time" db:"lease_max_time" msgpack:"lease_max_time"`
	LeaseMaxExtensions int           `json:"lease_max_extensions" db:"lease_max_extensions" msgpack:"lease_max_extensions"`
	CreatedUserId      string        `json:"created_user_id" db:"created_user_id" msgpack:"created_user_id"`
	CreatedAt          time.Time     `json:"created_at" db:"created_at" msgpack:"created_at"`
	UpdatedUserId      string        `json:"updated_user_id" db:"updated_user_id" msgpack:"updated_user_id"`
	UpdatedAt          hlc.Timestamp `json:"updated_at" db:"updated_at" msgpack:"updated_at"`
}

// LeaseDuration converts the pool's LeaseMaxTime into a time.Duration.
// The second return value reports whether leases are enabled at all.
func (p *PoolDefinition) LeaseDuration() (time.Duration, bool) {
	if p.LeaseMaxTime == 0 {
		return 0, false
	}
	if p.LeaseMaxTime < 0 {
		return 0, true // enabled, no timeout
	}
	return time.Duration(p.LeaseMaxTime) * time.Second, true
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
