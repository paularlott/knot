package model

import (
	"time"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/util/crypt"

	"github.com/google/uuid"
	"github.com/paularlott/knot/internal/log"
)

// Template Variable object
type TemplateVar struct {
	Id            string        `json:"templatevar_id" db:"templatevar_id,pk" msgpack:"templatevar_id"`
	Name          string        `json:"name" db:"name" msgpack:"name"`
	Zones         []string      `json:"zones" db:"zones,json" msgpack:"zones"`
	Value         string        `json:"value" db:"value" msgpack:"value"`
	Protected     bool          `json:"protected" db:"protected" msgpack:"protected"`
	Local         bool          `json:"local" db:"local" msgpack:"local"`
	Restricted    bool          `json:"restricted" db:"restricted" msgpack:"restricted"`
	IsDeleted     bool          `json:"is_deleted" db:"is_deleted" msgpack:"is_deleted"`
	IsManaged     bool          `json:"is_managed" db:"is_managed" msgpack:"is_managed"`
	CreatedUserId string        `json:"created_user_id" db:"created_user_id" msgpack:"created_user_id"`
	CreatedAt     time.Time     `json:"created_at" db:"created_at" msgpack:"created_at"`
	UpdatedUserId string        `json:"updated_user_id" db:"updated_user_id" msgpack:"updated_user_id"`
	UpdatedAt     hlc.Timestamp `json:"updated_at" db:"updated_at" msgpack:"updated_at"`
}

func NewTemplateVar(name string, zones []string, local bool, value string, protected bool, restricted bool, userId string) *TemplateVar {
	id, err := uuid.NewV7()
	if err != nil {
		log.Fatal(err.Error())
	}

	templateVar := &TemplateVar{
		Id:            id.String(),
		Name:          name,
		Zones:         zones,
		Local:         local,
		Value:         value,
		Protected:     protected,
		Restricted:    restricted,
		CreatedUserId: userId,
		CreatedAt:     time.Now().UTC(),
		UpdatedUserId: userId,
		UpdatedAt:     hlc.Now(),
	}

	return templateVar
}

func (templateVar *TemplateVar) DecryptSetValue(text string) {
	cfg := config.GetServerConfig()
	if templateVar.Protected {
		key := cfg.EncryptionKey

		if key == "" {
			log.Fatal("No encryption key set")
		}

		templateVar.Value = crypt.DecryptB64(key, text)
	}
}

func (templateVar *TemplateVar) GetValueEncrypted() string {
	cfg := config.GetServerConfig()
	if templateVar.Protected {
		key := cfg.EncryptionKey

		if key == "" {
			log.Fatal("No encryption key set")
		}

		return crypt.EncryptB64(key, templateVar.Value)
	} else {
		return templateVar.Value
	}
}
