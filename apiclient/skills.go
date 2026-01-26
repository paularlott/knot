package apiclient

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
