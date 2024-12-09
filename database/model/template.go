package model

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

const (
	REMOTE_SERVER_TEMPLATE_FETCH_HASH_INTERVAL = 30 * time.Second
)

// Template object
type Template struct {
	Id               string      `json:"template_id"`
	Name             string      `json:"name"`
	Description      string      `json:"description"`
	Hash             string      `json:"hash"`
	Job              string      `json:"job"`
	Volumes          string      `json:"volumes"`
	Groups           JSONDbArray `json:"groups"`
	LocalContainer   bool        `json:"local_container"`
	IsManual         bool        `json:"is_manual"`
	WithTerminal     bool        `json:"with_terminal"`
	WithVSCodeTunnel bool        `json:"with_vscode_tunnel"`
	WithCodeServer   bool        `json:"with_code_server"`
	CreatedUserId    string      `json:"created_user_id"`
	CreatedAt        time.Time   `json:"created_at"`
	UpdatedUserId    string      `json:"updated_user_id"`
	UpdatedAt        time.Time   `json:"updated_at"`
}

func NewTemplate(name string, description string, job string, volumes string, userId string, groups []string, localContainer bool, isManual bool, withTerminal bool, withVSCodeTunnel bool, withCodeServer bool) *Template {
	id, err := uuid.NewV7()
	if err != nil {
		log.Fatal().Msg(err.Error())
	}

	template := &Template{
		Id:               id.String(),
		Name:             name,
		Description:      description,
		Job:              job,
		Volumes:          volumes,
		Groups:           groups,
		CreatedUserId:    userId,
		LocalContainer:   localContainer,
		IsManual:         isManual,
		WithTerminal:     withTerminal,
		WithVSCodeTunnel: withVSCodeTunnel,
		WithCodeServer:   withCodeServer,
		CreatedAt:        time.Now().UTC(),
		UpdatedUserId:    userId,
		UpdatedAt:        time.Now().UTC(),
	}
	template.UpdateHash()

	return template
}

func (template *Template) GetVolumes(space *Space, user *User, variables *map[string]interface{}, applySpaceSizes bool) (*CSIVolumes, error) {
	return LoadVolumesFromYaml(template.Volumes, template, space, user, variables, applySpaceSizes)
}

func (template *Template) UpdateHash() {
	hash := md5.Sum([]byte(template.Job + template.Volumes + fmt.Sprintf("%t%t%t", template.WithTerminal, template.WithVSCodeTunnel, template.WithCodeServer)))
	template.Hash = hex.EncodeToString(hash[:])
}
