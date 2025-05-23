package apiclient

type TemplateVarValue struct {
	Name       string `json:"name"`
	Location   string `json:"location"`
	Local      bool   `json:"local"`
	Value      string `json:"value"`
	Protected  bool   `json:"protected"`
	Restricted bool   `json:"restricted"`
}

type TemplateVar struct {
	Id         string `json:"templatevar_id"`
	Name       string `json:"name"`
	Location   string `json:"location"`
	Local      bool   `json:"local"`
	Protected  bool   `json:"protected"`
	Restricted bool   `json:"restricted"`
}

type TemplateVarList struct {
	Count       int           `json:"count"`
	TemplateVar []TemplateVar `json:"variables"`
}

type TemplateVarCreateResponse struct {
	Status bool   `json:"status"`
	Id     string `json:"templatevar_id"`
}

func (c *ApiClient) GetTemplateVars() (*TemplateVarList, int, error) {
	response := &TemplateVarList{}

	code, err := c.httpClient.Get("/api/templatevars", response)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}

func (c *ApiClient) UpdateTemplateVar(templateVarId string, name string, location string, local bool, value string, protected bool, restricted bool) (int, error) {
	request := TemplateVarValue{
		Name:       name,
		Location:   location,
		Local:      local,
		Value:      value,
		Protected:  protected,
		Restricted: restricted,
	}

	return c.httpClient.Put("/api/templatevars/"+templateVarId, request, nil, 200)
}

func (c *ApiClient) CreateTemplateVar(name string, location string, local bool, value string, protected bool, restricted bool) (string, int, error) {
	request := TemplateVarValue{
		Name:       name,
		Location:   location,
		Local:      local,
		Value:      value,
		Protected:  protected,
		Restricted: restricted,
	}

	response := &TemplateVarCreateResponse{}

	code, err := c.httpClient.Post("/api/templatevars", request, response, 201)
	if err != nil {
		return "", code, err
	}

	return response.Id, code, nil
}

func (c *ApiClient) DeleteTemplateVar(templateVarId string) (int, error) {
	return c.httpClient.Delete("/api/templatevars/"+templateVarId, nil, nil, 200)
}

func (c *ApiClient) GetTemplateVar(templateVarId string) (*TemplateVarValue, int, error) {
	response := &TemplateVarValue{}

	code, err := c.httpClient.Get("/api/templatevars/"+templateVarId, response)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}
