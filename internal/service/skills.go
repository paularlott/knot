package service

import (
	"fmt"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
)

type SkillService struct{}

type SkillListOptions struct {
	FilterUserId         string
	User                 *model.User
	IncludeDeleted       bool
	CheckZoneRestriction bool
}

var skillService *SkillService

func GetSkillService() *SkillService {
	if skillService == nil {
		skillService = &SkillService{}
	}
	return skillService
}

func (s *SkillService) ListSkills(opts SkillListOptions) ([]*model.Skill, error) {
	db := database.GetInstance()
	skills, err := db.GetSkills()
	if err != nil {
		return nil, fmt.Errorf("failed to get skills: %v", err)
	}

	cfg := config.GetServerConfig()
	var result []*model.Skill

	for _, skill := range skills {
		if skill.IsDeleted && !opts.IncludeDeleted {
			continue
		}

		isUserSkills := opts.FilterUserId != ""

		if isUserSkills {
			if skill.UserId != opts.FilterUserId {
				continue
			}
		} else {
			if skill.UserId != "" {
				continue
			}

			if opts.User != nil && !opts.User.HasPermission(model.PermissionManageGlobalSkills) {
				if len(skill.Groups) > 0 && !opts.User.HasAnyGroup(&skill.Groups) {
					continue
				}
			}
		}

		if opts.CheckZoneRestriction && !skill.IsValidForZone(cfg.Zone) {
			continue
		}

		result = append(result, skill)
	}

	return result, nil
}
