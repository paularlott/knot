package model

import (
	"time"

	"github.com/google/uuid"
)

// Template object
type Template struct {
	Id string `json:"template_id"`
  Name string `json:"name"`
  Job string `json:"job"`
  CreatedUserId string `json:"created_user_id"`
  CreatedAt time.Time `json:"created_at"`
  UpdatedUserId string `json:"updated_user_id"`
  UpdatedAt time.Time `json:"updated_at"`
}

func NewTemplate(name string, job string, userId string) *Template {
  template := &Template{
    Id: uuid.New().String(),
    Name: name,
    Job: job,
    CreatedUserId: userId,
    CreatedAt: time.Now().UTC(),
    UpdatedUserId: userId,
    UpdatedAt: time.Now().UTC(),
  }

  return template
}
