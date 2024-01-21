package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// Template Variable object
type TemplateVar struct {
	Id string `json:"templatevar_id"`
  Name string `json:"name"`
  Value string `json:"value"`
  CreatedUserId string `json:"created_user_id"`
  CreatedAt time.Time `json:"created_at"`
  UpdatedUserId string `json:"updated_user_id"`
  UpdatedAt time.Time `json:"updated_at"`
}

func NewTemplateVar(name string, value string, userId string) *TemplateVar {
  id, err := uuid.NewV7()
  if err != nil {
    log.Fatal().Msg(err.Error())
  }

  templateVar := &TemplateVar{
    Id: id.String(),
    Name: name,
    Value: value,
    CreatedUserId: userId,
    CreatedAt: time.Now().UTC(),
    UpdatedUserId: userId,
    UpdatedAt: time.Now().UTC(),
  }

  return templateVar
}
