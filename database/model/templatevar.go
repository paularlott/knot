package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/paularlott/knot/util/crypt"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

// Template Variable object
type TemplateVar struct {
	Id            string    `json:"templatevar_id"`
	Name          string    `json:"name"`
	Location      string    `json:"location"`
	Value         string    `json:"value"`
	Protected     bool      `json:"protected"`
	Local         bool      `json:"local"`
	Restricted    bool      `json:"restricted"`
	CreatedUserId string    `json:"created_user_id"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedUserId string    `json:"updated_user_id"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func NewTemplateVar(name string, location string, local bool, value string, protected bool, restricted bool, userId string) *TemplateVar {
	id, err := uuid.NewV7()
	if err != nil {
		log.Fatal().Msg(err.Error())
	}

	templateVar := &TemplateVar{
		Id:            id.String(),
		Name:          name,
		Location:      location,
		Local:         local,
		Value:         value,
		Protected:     protected,
		Restricted:    restricted,
		CreatedUserId: userId,
		CreatedAt:     time.Now().UTC(),
		UpdatedUserId: userId,
		UpdatedAt:     time.Now().UTC(),
	}

	return templateVar
}

func (templateVar *TemplateVar) DecryptSetValue(text string) {
	if templateVar.Protected {
		key := viper.GetString("server.encrypt")

		if key == "" {
			log.Fatal().Msg("No encryption key set")
		}

		templateVar.Value = crypt.DecryptB64(key, text)
	}
}

func (templateVar *TemplateVar) GetValueEncrypted() string {
	if templateVar.Protected {
		key := viper.GetString("server.encrypt")

		if key == "" {
			log.Fatal().Msg("No encryption key set")
		}

		return crypt.EncryptB64(key, templateVar.Value)
	} else {
		return templateVar.Value
	}
}
