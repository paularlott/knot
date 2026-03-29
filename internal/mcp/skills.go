package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/database/model"
)

// BuildSkillsPrompt returns a skills section to append to the system prompt,
// or an empty string if the user has no accessible active skills.
func BuildSkillsPrompt(ctx context.Context, user *model.User) string {
	if user == nil {
		return ""
	}

	client := apiclient.NewMuxClient(user)

	var response apiclient.SkillList
	_, err := client.Do(ctx, "GET", "/api/skill", nil, &response)
	if err != nil {
		return ""
	}

	var active []apiclient.SkillInfo
	for _, skill := range response.Skills {
		if skill.Active {
			active = append(active, skill)
		}
	}

	if len(active) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n\n## Available Skills\n\n")
	sb.WriteString("Use `get_skill` to retrieve full skill content before following any procedure.\n\n")
	for _, skill := range active {
		sb.WriteString(fmt.Sprintf("- **%s**: %s\n", skill.Name, skill.Description))
	}

	return sb.String()
}
