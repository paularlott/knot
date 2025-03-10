package apiclient

type TemplateCreateRequest struct {
	Name             string               `json:"name"`
	Job              string               `json:"job"`
	Description      string               `json:"description"`
	Volumes          string               `json:"volumes"`
	Groups           []string             `json:"groups"`
	LocalContainer   bool                 `json:"local_container"`
	IsManual         bool                 `json:"is_manual"`
	WithTerminal     bool                 `json:"with_terminal"`
	WithVSCodeTunnel bool                 `json:"with_vscode_tunnel"`
	WithCodeServer   bool                 `json:"with_code_server"`
	WithSSH          bool                 `json:"with_ssh"`
	ScheduleEnabled  bool                 `json:"schedule_enabled"`
	Schedule         []TemplateDetailsDay `json:"schedule"`
	ComputeUnits     uint32               `json:"compute_units"`
	StorageUnits     uint32               `json:"storage_units"`
	Locations        []string             `json:"locations"`
}

type TemplateUpdateRequest struct {
	Name             string               `json:"name"`
	Job              string               `json:"job"`
	Description      string               `json:"description"`
	Volumes          string               `json:"volumes"`
	Groups           []string             `json:"groups"`
	WithTerminal     bool                 `json:"with_terminal"`
	WithVSCodeTunnel bool                 `json:"with_vscode_tunnel"`
	WithCodeServer   bool                 `json:"with_code_server"`
	WithSSH          bool                 `json:"with_ssh"`
	ScheduleEnabled  bool                 `json:"schedule_enabled"`
	Schedule         []TemplateDetailsDay `json:"schedule"`
	ComputeUnits     uint32               `json:"compute_units"`
	StorageUnits     uint32               `json:"storage_units"`
	Locations        []string             `json:"locations"`
}

type TemplateCreateResponse struct {
	Status bool   `json:"status"`
	Id     string `json:"template_id"`
}

type TemplateInfo struct {
	Id              string               `json:"template_id"`
	Name            string               `json:"name"`
	Description     string               `json:"description"`
	Usage           int                  `json:"usage"`
	Deployed        int                  `json:"deployed"`
	Groups          []string             `json:"groups"`
	LocalContainer  bool                 `json:"local_container"`
	IsManual        bool                 `json:"is_manual"`
	ScheduleEnabled bool                 `json:"schedule_enabled"`
	ComputeUnits    uint32               `json:"compute_units"`
	StorageUnits    uint32               `json:"storage_units"`
	Schedule        []TemplateDetailsDay `json:"schedule"`
	Locations       []string             `json:"locations"`
}

type TemplateList struct {
	Count     int            `json:"count"`
	Templates []TemplateInfo `json:"templates"`
}

type TemplateDetailsDay struct {
	Enabled bool   `json:"enabled"`
	From    string `json:"from"`
	To      string `json:"to"`
}

type TemplateDetails struct {
	Name             string               `json:"name"`
	Job              string               `json:"job"`
	Description      string               `json:"description"`
	Volumes          string               `json:"volumes"`
	Usage            int                  `json:"usage"`
	Hash             string               `json:"hash"`
	Deployed         int                  `json:"deployed"`
	Groups           []string             `json:"groups"`
	LocalContainer   bool                 `json:"local_container"`
	IsManual         bool                 `json:"is_manual"`
	WithTerminal     bool                 `json:"with_terminal"`
	WithVSCodeTunnel bool                 `json:"with_vscode_tunnel"`
	WithCodeServer   bool                 `json:"with_code_server"`
	WithSSH          bool                 `json:"with_ssh"`
	ComputeUnits     uint32               `json:"compute_units"`
	StorageUnits     uint32               `json:"storage_units"`
	ScheduleEnabled  bool                 `json:"schedule_enabled"`
	Schedule         []TemplateDetailsDay `json:"schedule"`
	Locations        []string             `json:"locations"`
}

func (c *ApiClient) GetTemplates() (*TemplateList, int, error) {
	response := &TemplateList{}

	code, err := c.httpClient.Get("/api/templates", response)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}

func (c *ApiClient) UpdateTemplate(templateId string, name string, job string, description string, volumes string, groups []string, withTerminal bool, withVSCodeTunnel bool, withCodeServer bool, withSSH bool, computeUnits uint32, storageUnits uint32, scheduleEnabled bool, schedule *[]TemplateDetailsDay, locations []string) (int, error) {
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
		ComputeUnits:     computeUnits,
		StorageUnits:     storageUnits,
		Locations:        locations,
	}

	if schedule == nil || !scheduleEnabled {
		request.ScheduleEnabled = false
		request.Schedule = nil
	} else {
		request.ScheduleEnabled = true
		request.Schedule = *schedule
	}

	return c.httpClient.Put("/api/templates/"+templateId, &request, nil, 200)
}

func (c *ApiClient) CreateTemplate(name string, job string, description string, volumes string, groups []string, localContainer bool, IsManual bool, withTerminal bool, withVSCodeTunnel bool, withCodeServer bool, withSSH bool, computeUnits uint32, storageUnits uint32, scheduleEnabled bool, schedule *[]TemplateDetailsDay, locations []string) (string, int, error) {
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
		ComputeUnits:     computeUnits,
		StorageUnits:     storageUnits,
		Locations:        locations,
	}

	if schedule == nil || !scheduleEnabled {
		request.ScheduleEnabled = false
		request.Schedule = nil
	} else {
		request.ScheduleEnabled = true
		request.Schedule = *schedule
	}

	response := &TemplateCreateResponse{}

	code, err := c.httpClient.Post("/api/templates", &request, &response, 201)
	if err != nil {
		return "", code, err
	}

	return response.Id, code, nil
}

func (c *ApiClient) DeleteTemplate(templateId string) (int, error) {
	return c.httpClient.Delete("/api/templates/"+templateId, nil, nil, 200)
}

func (c *ApiClient) GetTemplate(templateId string) (*TemplateDetails, int, error) {
	response := &TemplateDetails{}

	code, err := c.httpClient.Get("/api/templates/"+templateId, response)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}
