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

	db := database.GetInstance()
	if db == nil {
		return ""
	}

	skills, err := db.GetSkills()
	if err != nil {
		return ""
	}

	currentZone := config.GetServerConfig().Zone

	type skillEntry struct {
		name        string
		description string
		isUser      bool
	}

	// Collect by name — user skills override global skills with the same name
	byName := make(map[string]skillEntry)

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

		entry := skillEntry{
			name:        skill.Name,
			description: skill.Description,
			isUser:      skill.IsUserSkill(),
		}

		if existing, ok := byName[skill.Name]; ok {
			// Only replace if new is user skill and existing is global
			if entry.isUser && !existing.isUser {
				byName[skill.Name] = entry
			}
		} else {
			byName[skill.Name] = entry
		}
	}

	if len(byName) == 0 {
		return ""
	}

	active := make([]skillEntry, 0, len(byName))
	for _, entry := range byName {
		active = append(active, entry)
	}

	sort.Slice(active, func(i, j int) bool {
		return active[i].name < active[j].name
	})

	var sb strings.Builder
	sb.WriteString("\n\n## Available Skills\n\n")
	sb.WriteString("Retrieve full content with `execute_tool(name=\"get_skill\", arguments={\"name\": \"<skill-name>\"})` before following any procedure.\n\n")
	for _, skill := range active {
		sb.WriteString(fmt.Sprintf("- **%s**: %s\n", skill.name, skill.description))
	}

	return sb.String()
}
