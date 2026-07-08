package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/log"
)

// Command is a user-invokable slash command stored in the database. Mirrors
// the shape of a lmchatkit slash-command markdown file (name, description,
// argument-hint, allowed-tools, body) plus the ownership/ACL fields used by
// knot (UserId for user/global, Groups, Zones, Active). The Body is the raw
// markdown with an optional $ARGUMENTS placeholder substituted at render time.
type Command struct {
	Id            string        `json:"command_id" db:"command_id,pk"`
	UserId        string        `json:"user_id" db:"user_id"`
	Name          string        `json:"name" db:"name"`
	Description   string        `json:"description" db:"description"`
	ArgumentHint  string        `json:"argument_hint" db:"argument_hint"`
	AllowedTools  []string      `json:"allowed_tools" db:"allowed_tools,json"`
	Body          string        `json:"body" db:"body"`
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

func NewCommand(
	name string,
	description string,
	argumentHint string,
	allowedTools []string,
	body string,
	groups []string,
	zones []string,
	ownerUserId string,
	createdUserId string,
) *Command {
	id, err := uuid.NewV7()
	if err != nil {
		log.Fatal(err.Error())
	}

	if allowedTools == nil {
		allowedTools = []string{}
	}
	if groups == nil {
		groups = []string{}
	}
	if zones == nil {
		zones = []string{}
	}

	return &Command{
		Id:            id.String(),
		UserId:        ownerUserId,
		Name:          name,
		Description:   description,
		ArgumentHint:  argumentHint,
		AllowedTools:  allowedTools,
		Body:          body,
		Groups:        groups,
		Zones:         zones,
		Active:        true,
		CreatedUserId: createdUserId,
		CreatedAt:     time.Now().UTC(),
		UpdatedUserId: createdUserId,
		UpdatedAt:     hlc.Now(),
	}
}

func (command *Command) IsValidForZone(zone string) bool {
	if len(command.Zones) == 0 {
		return true
	}

	for _, z := range command.Zones {
		if len(z) > 0 && z[0] == '!' && z[1:] == zone {
			return false
		}
	}

	for _, z := range command.Zones {
		if len(z) > 0 && z[0] != '!' && z == zone {
			return true
		}
	}

	return false
}

func (command *Command) IsGlobalCommand() bool {
	return command.UserId == ""
}

func (command *Command) IsUserCommand() bool {
	return command.UserId != ""
}
