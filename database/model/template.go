package model

import (
	"crypto/md5"
	"encoding/hex"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

const MANUAL_TEMPLATE_ID = "00000000-0000-0000-0000-000000000000"

// Template object
type Template struct {
	Id string `json:"template_id"`
  Name string `json:"name"`
  Hash string `json:"hash"`
  Job string `json:"job"`
  Volumes string `json:"volumes"`
  Groups JSONDbArray `json:"groups"`
  CreatedUserId string `json:"created_user_id"`
  CreatedAt time.Time `json:"created_at"`
  UpdatedUserId string `json:"updated_user_id"`
  UpdatedAt time.Time `json:"updated_at"`
}

func NewTemplate(name string, job string, volumes string, userId string, groups []string) *Template {
  id, err := uuid.NewV7()
  if err != nil {
    log.Fatal().Msg(err.Error())
  }

  template := &Template{
    Id: id.String(),
    Name: name,
    Job: job,
    Volumes: volumes,
    Groups: groups,
    CreatedUserId: userId,
    CreatedAt: time.Now().UTC(),
    UpdatedUserId: userId,
    UpdatedAt: time.Now().UTC(),
  }
  template.UpdateHash()

  return template
}

func (template *Template) GetVolumes(space *Space, user *User, variables *map[string]interface{}) (*Volumes, error) {
  return LoadVolumesFromYaml(template.Volumes, template, space, user, variables)
}

func (template *Template) UpdateHash() {
  hash := md5.Sum([]byte(template.Job + template.Volumes))
  template.Hash = hex.EncodeToString(hash[:])
}
