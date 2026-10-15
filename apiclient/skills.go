package apiclient

import "context"

type SkillList struct {
	Count  int         `json:"count" msgpack:"count"`
	Skills []SkillInfo `json:"skills" msgpack:"skills"`
}

type SkillInfo struct {
	Id          string   `json:"skill_id" msgpack:"skill_id"`
	UserId      string   `json:"user_id" msgpack:"user_id"`
	Name        string   `json:"name" msgpack:"name"`
	Description string   `json:"description" msgpack:"description"`
	Groups      []string `json:"groups" msgpack:"groups"`
	Zones       []string `json:"zones" msgpack:"zones"`
	Active      bool     `json:"active" msgpack:"active"`
	IsManaged   bool     `json:"is_managed" msgpack:"is_managed"`
}

type SkillDetails struct {
	Id          string   `json:"skill_id" msgpack:"skill_id"`
	UserId      string   `json:"user_id" msgpack:"user_id"`
	Name        string   `json:"name" msgpack:"name"`
	Description string   `json:"description" msgpack:"description"`
	Content     string   `json:"content" msgpack:"content"`
	Groups      []string `json:"groups" msgpack:"groups"`
	Zones       []string `json:"zones" msgpack:"zones"`
	Active      bool     `json:"active" msgpack:"active"`
	IsManaged   bool     `json:"is_managed" msgpack:"is_managed"`
}

type SkillCreateRequest struct {
	UserId  string   `json:"user_id" msgpack:"user_id"`
	Content string   `json:"content" msgpack:"content"`
	Groups  []string `json:"groups" msgpack:"groups"`
	Zones   []string `json:"zones" msgpack:"zones"`
	Active  bool     `json:"active" msgpack:"active"`
}

type SkillUpdateRequest struct {
	Content string   `json:"content" msgpack:"content"`
	Groups  []string `json:"groups" msgpack:"groups"`
	Zones   []string `json:"zones" msgpack:"zones"`
	Active  bool     `json:"active" msgpack:"active"`
}

type SkillCreateResponse struct {
	Status bool   `json:"status" msgpack:"status"`
	Id     string `json:"skill_id" msgpack:"skill_id"`
}

type SkillUpdateResponse struct {
	Status bool `json:"status" msgpack:"status"`
}

type SkillSearchResult struct {
	Skill string  `json:"skill" msgpack:"skill"`
	Score float64 `json:"score" msgpack:"score"`
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

func (c *ApiClient) UpdateSkill(ctx context.Context, nameOrId string, req *SkillUpdateRequest) (*SkillUpdateResponse, error) {
	var resp SkillUpdateResponse
	_, err := c.httpClient.Put(ctx, "/api/skill/"+nameOrId, req, &resp, 200)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *ApiClient) DeleteSkill(ctx context.Context, nameOrId string) error {
	_, err := c.httpClient.Delete(ctx, "/api/skill/"+nameOrId, nil, nil, 0)
	return err
}
