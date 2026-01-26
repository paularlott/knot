package apiclient

import "context"

type SkillList struct {
	Count  int         `json:"count"`
	Skills []SkillInfo `json:"skills"`
}

type SkillInfo struct {
	Id          string   `json:"skill_id"`
	UserId      string   `json:"user_id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Groups      []string `json:"groups"`
	Zones       []string `json:"zones"`
	Active      bool     `json:"active"`
	IsManaged   bool     `json:"is_managed"`
}

type SkillDetails struct {
	Id          string   `json:"skill_id"`
	UserId      string   `json:"user_id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Content     string   `json:"content"`
	Groups      []string `json:"groups"`
	Zones       []string `json:"zones"`
	Active      bool     `json:"active"`
	IsManaged   bool     `json:"is_managed"`
}

type SkillCreateRequest struct {
	UserId  string   `json:"user_id"`
	Content string   `json:"content"`
	Groups  []string `json:"groups"`
	Zones   []string `json:"zones"`
	Active  bool     `json:"active"`
}

type SkillUpdateRequest struct {
	Content string   `json:"content"`
	Groups  []string `json:"groups"`
	Zones   []string `json:"zones"`
	Active  bool     `json:"active"`
}

type SkillCreateResponse struct {
	Status bool   `json:"status"`
	Id     string `json:"skill_id"`
}

func (c *ApiClient) GetSkills(ctx context.Context) (*SkillList, error) {
	var skills SkillList
	_, err := c.httpClient.Get(ctx, "/api/skill", &skills)
	return &skills, err
}

func (c *ApiClient) GetSkill(ctx context.Context, nameOrId string) (*SkillDetails, error) {
	var skill SkillDetails
	_, err := c.httpClient.Get(ctx, "/api/skill/"+nameOrId, &skill)
	return &skill, err
}

func (c *ApiClient) CreateSkill(ctx context.Context, req *SkillCreateRequest) (*SkillCreateResponse, error) {
	var resp SkillCreateResponse
	_, err := c.httpClient.Post(ctx, "/api/skill", req, &resp, 201)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *ApiClient) UpdateSkill(ctx context.Context, nameOrId string, req *SkillUpdateRequest) error {
	_, err := c.httpClient.Put(ctx, "/api/skill/"+nameOrId, req, nil, 200)
	return err
}

func (c *ApiClient) DeleteSkill(ctx context.Context, nameOrId string) error {
	_, err := c.httpClient.Delete(ctx, "/api/skill/"+nameOrId, nil, nil, 0)
	return err
}
