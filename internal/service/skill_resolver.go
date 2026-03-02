package service

import (
	"fmt"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
)

func ResolveSkillByName(name string, userId string) (*model.Skill, error) {
	db := database.GetInstance()
	cfg := config.GetServerConfig()

	if userId != "" {
		skills, err := db.GetSkillsByNameAndUser(name, userId)
		if err == nil {
			for _, skill := range skills {
				if !skill.IsDeleted && skill.IsValidForZone(cfg.Zone) {
					return skill, nil
				}
			}
		}
	}

	skills, err := db.GetSkillsByName(name)
	if err != nil {
		return nil, fmt.Errorf("skill not found")
	}

	for _, skill := range skills {
		if skill.IsDeleted {
			continue
		}

		if !skill.IsValidForZone(cfg.Zone) {
			continue
		}

		return skill, nil
	}

	return nil, fmt.Errorf("skill not found")
}

func CanUserAccessSkill(user *model.User, skill *model.Skill) bool {
	if skill.IsUserSkill() {
		if skill.UserId == user.Id {
			return user.HasPermission(model.PermissionManageOwnSkills)
		}
		return false
	}

	if !user.HasPermission(model.PermissionManageGlobalSkills) {
		return false
	}

	if !user.HasPermission(model.PermissionManageGlobalSkills) {
		if len(skill.Groups) > 0 && !user.HasAnyGroup(&skill.Groups) {
			return false
		}
	}

	return true
}
