package apiclient

type TemplateCreateRequest struct {
	Name             string   `json:"name"`
	Job              string   `json:"job"`
	Description      string   `json:"description"`
	Volumes          string   `json:"volumes"`
	Groups           []string `json:"groups"`
	LocalContainer   bool     `json:"local_container"`
	IsManual         bool     `json:"is_manual"`
	WithTerminal     bool     `json:"with_terminal"`
	WithVSCodeTunnel bool     `json:"with_vscode_tunnel"`
	WithCodeServer   bool     `json:"with_code_server"`
	WithSSH          bool     `json:"with_ssh"`
}

type TemplateUpdateRequest struct {
	Name             string   `json:"name"`
	Job              string   `json:"job"`
	Description      string   `json:"description"`
	Volumes          string   `json:"volumes"`
	Groups           []string `json:"groups"`
	WithTerminal     bool     `json:"with_terminal"`
	WithVSCodeTunnel bool     `json:"with_vscode_tunnel"`
	WithCodeServer   bool     `json:"with_code_server"`
	WithSSH          bool     `json:"with_ssh"`
}

type TemplateCreateResponse struct {
	Status bool   `json:"status"`
	Id     string `json:"template_id"`
}

type TemplateInfo struct {
	Id             string   `json:"template_id"`
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Usage          int      `json:"usage"`
	Deployed       int      `json:"deployed"`
	Groups         []string `json:"groups"`
	LocalContainer bool     `json:"local_container"`
	IsManual       bool     `json:"is_manual"`
}

type TemplateList struct {
	Count     int            `json:"count"`
	Templates []TemplateInfo `json:"templates"`
}

type TemplateDetails struct {
	Name             string                   `json:"name"`
	Job              string                   `json:"job"`
	Description      string                   `json:"description"`
	Volumes          string                   `json:"volumes"`
	Usage            int                      `json:"usage"`
	Hash             string                   `json:"hash"`
	Deployed         int                      `json:"deployed"`
	Groups           []string                 `json:"groups"`
	VolumeSizes      []map[string]interface{} `json:"volume_sizes"`
	LocalContainer   bool                     `json:"local_container"`
	IsManual         bool                     `json:"is_manual"`
	WithTerminal     bool                     `json:"with_terminal"`
	WithVSCodeTunnel bool                     `json:"with_vscode_tunnel"`
	WithCodeServer   bool                     `json:"with_code_server"`
	WithSSH          bool                     `json:"with_ssh"`
}

func (c *ApiClient) GetTemplates() (*TemplateList, int, error) {
	response := &TemplateList{}

	code, err := c.httpClient.Get("/api/v1/templates", response)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}

func (c *ApiClient) UpdateTemplate(templateId string, name string, job string, description string, volumes string, groups []string, withTerminal bool, withVSCodeTunnel bool, withCodeServer bool, withSSH bool) (int, error) {
	request := TemplateUpdateRequest{
		Name:             name,
		Job:              job,
		Description:      description,
		Volumes:          volumes,
		Groups:           groups,
		WithTerminal:     withTerminal,
		WithVSCodeTunnel: withVSCodeTunnel,
		WithCodeServer:   withCodeServer,
		WithSSH:          withSSH,
	}

	return c.httpClient.Put("/api/v1/templates/"+templateId, &request, nil, 200)
}

func (c *ApiClient) CreateTemplate(name string, job string, description string, volumes string, groups []string, localContainer bool, IsManual bool, withTerminal bool, withVSCodeTunnel bool, withCodeServer bool, withSSH bool) (string, int, error) {
	request := TemplateCreateRequest{
		Name:             name,
		Job:              job,
		Description:      description,
		Volumes:          volumes,
		Groups:           groups,
		LocalContainer:   localContainer,
		IsManual:         IsManual,
		WithTerminal:     withTerminal,
		WithVSCodeTunnel: withVSCodeTunnel,
		WithCodeServer:   withCodeServer,
		WithSSH:          withSSH,
	}

	response := &TemplateCreateResponse{}

	code, err := c.httpClient.Post("/api/v1/templates", &request, &response, 201)
	if err != nil {
		return "", code, err
	}

	return response.Id, code, nil
}

func (c *ApiClient) DeleteTemplate(templateId string) (int, error) {
	return c.httpClient.Delete("/api/v1/templates/"+templateId, nil, nil, 200)
}

func (c *ApiClient) GetTemplate(templateId string) (*TemplateDetails, int, error) {
	response := &TemplateDetails{}

	code, err := c.httpClient.Get("/api/v1/templates/"+templateId, response)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}
