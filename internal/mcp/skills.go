package mcp

import (
	"fmt"
	"sort"
	"strings"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
)

// GetAccessibleSkills returns the active, zone-valid, ACL-filtered skills for
// the user (global + own), with user skills overriding globals of the same
// name. Returns nil if the user is nil or the database is unreachable.
func GetAccessibleSkills(user *model.User) []*model.Skill {
	if user == nil {
		return nil
	}

	db := database.GetInstance()
	if db == nil {
		return nil
	}

	skills, err := db.GetSkills()
	if err != nil {
		return nil
	}

	currentZone := config.GetServerConfig().Zone

	byName := make(map[string]*model.Skill)
	for _, skill := range skills {
		if !skill.Active || skill.IsDeleted {
			continue
		}
		if !skill.IsValidForZone(currentZone) {
			continue
		}
		if !service.CanUserAccessSkill(user, skill) {
			continue
		}
		if existing, ok := byName[skill.Name]; ok {
			if skill.IsUserSkill() && !existing.IsUserSkill() {
				byName[skill.Name] = skill
			}
		} else {
			byName[skill.Name] = skill
		}
	}

	if len(byName) == 0 {
		return nil
	}

	result := make([]*model.Skill, 0, len(byName))
	for _, s := range byName {
		result = append(result, s)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// BuildSkillsPrompt returns a skills section to append to the system prompt,
// or an empty string if the user has no accessible active skills.
//
// Skills are fetched directly from the database (not via the management API)
// so that both global and user-owned skills are included, zone restrictions
// are enforced, and group-based access control is applied.
func BuildSkillsPrompt(user *model.User) string {
	if user == nil {
		return ""
	}

	skills := GetAccessibleSkills(user)
	if len(skills) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n\n## Available Skills\n\n")
	sb.WriteString("Retrieve full content with `execute_tool(name=\"get_skill\", arguments={\"name\": \"<skill-name>\"})` before following any procedure.\n\n")
	for _, skill := range skills {
		sb.WriteString(fmt.Sprintf("- **%s**: %s\n", skill.Name, skill.Description))
	}

	return sb.String()
}
