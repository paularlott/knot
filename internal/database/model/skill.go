package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/log"
)

type Skill struct {
	Id            string        `json:"skill_id" db:"skill_id,pk"`
	UserId        string        `json:"user_id" db:"user_id"`
	Name          string        `json:"name" db:"name"`
	Description   string        `json:"description" db:"description"`
	Content       string        `json:"content" db:"content"`
	Groups        []string      `json:"groups" db:"groups,json"`
	Zones         []string      `json:"zones" db:"zones,json"`
	Active        bool          `json:"active" db:"active"`
	IsDeleted     bool          `json:"is_deleted" db:"is_deleted"`
	IsManaged     bool          `json:"is_managed" db:"is_managed"`
	CreatedUserId string        `json:"created_user_id" db:"created_user_id"`
	CreatedAt     time.Time     `json:"created_at" db:"created_at"`
	UpdatedUserId string        `json:"updated_user_id" db:"updated_user_id"`
	UpdatedAt     hlc.Timestamp `json:"updated_at" db:"updated_at"`
}

func NewSkill(
	name string,
	description string,
	content string,
	groups []string,
	zones []string,
	ownerUserId string,
	createdUserId string,
) *Skill {
	id, err := uuid.NewV7()
	if err != nil {
		log.Fatal(err.Error())
	}

	return &Skill{
		Id:            id.String(),
		UserId:        ownerUserId,
		Name:          name,
		Description:   description,
		Content:       content,
		Groups:        groups,
		Zones:         zones,
		Active:        true,
		CreatedUserId: createdUserId,
		CreatedAt:     time.Now().UTC(),
		UpdatedUserId: createdUserId,
		UpdatedAt:     hlc.Now(),
	}
}

func (skill *Skill) IsValidForZone(zone string) bool {
	if len(skill.Zones) == 0 {
		return true
	}

	for _, z := range skill.Zones {
		if len(z) > 0 && z[0] == '!' && z[1:] == zone {
			return false
		}
	}

	for _, z := range skill.Zones {
		if len(z) > 0 && z[0] != '!' && z == zone {
			return true
		}
	}

	return false
}

func (skill *Skill) IsGlobalSkill() bool {
	return skill.UserId == ""
}

func (skill *Skill) IsUserSkill() bool {
	return skill.UserId != ""
}
