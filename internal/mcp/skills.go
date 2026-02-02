package mcp

import (
	"context"
	_ "embed"
	"fmt"
	"net/url"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/mcp"
)

const ToolNameFindSkill = "find_skill"

// getSkillsTool returns the find_skill tool if user has accessible skills
func getSkillsTool(ctx context.Context, user *model.User) *mcp.MCPTool {
	if user == nil {
		return nil
	}

	client := apiclient.NewMuxClient(user)

	var response apiclient.SkillList
	_, err := client.Do(ctx, "GET", "/api/skill", nil, &response)
	if err != nil {
		return nil
	}

	// Check if user has any active skills
	for _, skill := range response.Skills {
		if skill.Active {
			tool := mcp.NewTool(ToolNameFindSkill, "Access instructions, guides, workflows, and scripts for completing specific tasks. Search by topic (e.g., \"deploy\", \"git\", \"testing\") to get step-by-step procedures with examples. Returns full content with relevance scores.",
				mcp.String("name", "Find the exact skill by name (optional if query given)"),
				mcp.String("query", "Find skills by name/description (optional if name given)"),
			).ToMCPTool()
			return &tool
		}
	}

	return nil
}

// executeSkillsTool executes the find_skill tool
func executeSkillsTool(ctx context.Context, user *model.User, params map[string]interface{}) (interface{}, error) {
	query, _ := params["query"].(string)
	skillName, _ := params["name"].(string)

	// If both name and query are provided, try name first, then search as fallback
	if skillName != "" && query != "" {
		result, err := getSkillByName(ctx, user, skillName)
		if err == nil {
			return result, nil
		}
		// Name lookup failed, fall back to search
		return searchSkills(ctx, user, query)
	}

	// If name is provided, get specific skill
	if skillName != "" {
		return getSkillByName(ctx, user, skillName)
	}

	// If query is provided, search skills
	if query != "" {
		return searchSkills(ctx, user, query)
	}

	// Otherwise list all accessible skills (for discovery)
	return listSkills(ctx, user)
}

func listSkills(ctx context.Context, user *model.User) (*mcp.ToolResponse, error) {
	client := apiclient.NewMuxClient(user)

	var response apiclient.SkillList
	_, err := client.Do(ctx, "GET", "/api/skill", nil, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to list skills: %v", err)
	}

	skills := make([]map[string]interface{}, 0, len(response.Skills))

	// Add database skills (only active ones)
	for _, skill := range response.Skills {
		if skill.Active {
			skills = append(skills, map[string]interface{}{
				"name":        skill.Name,
				"description": skill.Description,
			})
		}
	}

	result := map[string]interface{}{
		"action": "list",
		"count":  len(skills),
		"skills": skills,
	}

	return mcp.NewToolResponseMulti(
		mcp.NewToolResponseJSON(result),
		mcp.NewToolResponseStructured(result),
	), nil
}

func searchSkills(ctx context.Context, user *model.User, query string) (*mcp.ToolResponse, error) {
	client := apiclient.NewMuxClient(user)

	var results []apiclient.SkillSearchResult
	_, err := client.Do(ctx, "GET", fmt.Sprintf("/api/skill/search?q=%s", url.QueryEscape(query)), nil, &results)
	if err != nil {
		return nil, fmt.Errorf("failed to search skills: %v", err)
	}

	// Return JSON only - structured response requires an object
	return mcp.NewToolResponseJSON(results), nil
}

func getSkillByName(ctx context.Context, user *model.User, name string) (*mcp.ToolResponse, error) {
	// Get from database
	client := apiclient.NewMuxClient(user)

	var skill apiclient.SkillDetails
	_, err := client.Do(ctx, "GET", fmt.Sprintf("/api/skill/%s", name), nil, &skill)
	if err != nil {
		return nil, fmt.Errorf("skill not found: %s", name)
	}

	// Check if content is empty (shouldn't happen but handle it)
	if skill.Content == "" {
		return nil, fmt.Errorf("skill content is empty: %s", name)
	}

	// Return same structure as search results, with perfect score
	result := apiclient.SkillSearchResult{
		Skill: skill.Content,
		Score: 1.0,
	}
	return mcp.NewToolResponseJSON(result), nil
}
