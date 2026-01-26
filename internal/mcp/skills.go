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

type SkillInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// getSkillsTool returns the skills tool if user has accessible skills
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
			return &mcp.MCPTool{
				Name:        "skills",
				Description: "Access knowledge base/skills for guides and best practices. Call without parameters to list all, with 'query' to search, or with 'name' to get specific content.",
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query": map[string]interface{}{
							"type":        "string",
							"description": "Search query to find skills by name or description (optional)",
						},
						"name": map[string]interface{}{
							"type":        "string",
							"description": "Skill name to retrieve specific content (optional)",
						},
					},
				},
				Keywords: []string{"skills", "knowledge", "guides", "documentation"},
			}
		}
	}

	return nil
}

// executeSkillsTool executes the skills tool
func executeSkillsTool(ctx context.Context, user *model.User, params map[string]interface{}) (interface{}, error) {
	query, _ := params["query"].(string)
	skillName, _ := params["name"].(string)

	// If query is provided, search skills (prioritize search over name)
	if query != "" {
		return searchSkills(ctx, user, query)
	}

	// If name is provided, get specific skill
	if skillName != "" {
		return getSkillByName(ctx, user, skillName)
	}

	// Otherwise list all accessible skills
	return listSkills(ctx, user)
}

func listSkills(ctx context.Context, user *model.User) (*mcp.ToolResponse, error) {
	client := apiclient.NewMuxClient(user)

	var response apiclient.SkillList
	_, err := client.Do(ctx, "GET", "/api/skill", nil, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to list skills: %v", err)
	}

	skills := make([]SkillInfo, 0, len(response.Skills))

	// Add database skills (only active ones)
	for _, skill := range response.Skills {
		if skill.Active {
			skills = append(skills, SkillInfo{
				Name:        skill.Name,
				Description: skill.Description,
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

	var response apiclient.SkillList
	_, err := client.Do(ctx, "GET", fmt.Sprintf("/api/skill/search?q=%s", url.QueryEscape(query)), nil, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to search skills: %v", err)
	}

	skills := make([]SkillInfo, 0, len(response.Skills))
	for _, skill := range response.Skills {
		if skill.Active {
			skills = append(skills, SkillInfo{
				Name:        skill.Name,
				Description: skill.Description,
			})
		}
	}

	result := map[string]interface{}{
		"action": "search",
		"query":  query,
		"count":  len(skills),
		"skills": skills,
	}

	return mcp.NewToolResponseMulti(
		mcp.NewToolResponseJSON(result),
		mcp.NewToolResponseStructured(result),
	), nil
}

func getSkillByName(ctx context.Context, user *model.User, name string) (*mcp.ToolResponse, error) {
	// Get from database
	client := apiclient.NewMuxClient(user)

	var skill apiclient.SkillDetails
	_, err := client.Do(ctx, "GET", fmt.Sprintf("/api/skill/%s", name), nil, &skill)
	if err != nil {
		return nil, fmt.Errorf("skill not found: %s", name)
	}

	return createContentResponse(skill.Name, skill.Content), nil
}

func createContentResponse(name, content string) *mcp.ToolResponse {
	result := map[string]interface{}{
		"name":    name,
		"content": content,
	}
	return mcp.NewToolResponseMulti(
		mcp.NewToolResponseJSON(result),
		mcp.NewToolResponseStructured(result),
	)
}
