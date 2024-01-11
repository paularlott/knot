package model

import (
	"time"

	"github.com/google/uuid"
)

// Group object
type Group struct {
	Id string `json:"group_id"`
  Name string `json:"name"`
  CreatedUserId string `json:"created_user_id"`
  CreatedAt time.Time `json:"created_at"`
  UpdatedUserId string `json:"updated_user_id"`
  UpdatedAt time.Time `json:"updated_at"`
}

func NewGroup(name string, userId string) *Group {
  group := &Group{
    Id: uuid.New().String(),
    Name: name,
    CreatedUserId: userId,
    CreatedAt: time.Now().UTC(),
    UpdatedUserId: userId,
    UpdatedAt: time.Now().UTC(),
  }

  return group
}
