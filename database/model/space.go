package model

import (
	"time"

	"github.com/google/uuid"
)

// Space object
type Space struct {
	Id string `json:"space_id"`
  UserId string `json:"user_id"`
  AccessToken string `json:"access_token"`
  TemplateId string `json:"template_id"`
  Name string `json:"name"`
  AgentURL string `json:"agent_url"`
  IsRunning bool `json:"is_running"`
  HasVSCode bool `json:"has_vscode"`
  HasSSH bool `json:"has_ssh"`
  LastSeen time.Time `json:"last_seen"`
  CreatedAt time.Time `json:"created_at"`
  UpdatedAt time.Time `json:"updated_at"`
}

func NewSpace(name string, userId string, agentURL string, templateId string) *Space {
  space := &Space{
    Id: uuid.New().String(),
    AccessToken: "",
    UserId: userId,
    TemplateId: templateId,
    Name: name,
    AgentURL: agentURL,
    IsRunning: false,
    HasVSCode: false,
    HasSSH: false,
    LastSeen: time.Now().UTC(),
    CreatedAt: time.Now().UTC(),
    UpdatedAt: time.Now().UTC(),
  }

  return space
}
