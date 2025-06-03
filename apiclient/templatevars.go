package apiclient

import "context"

type TemplateVarValue struct {
	Name       string `json:"name"`
	Location   string `json:"location"`
	Local      bool   `json:"local"`
	Value      string `json:"value"`
	Protected  bool   `json:"protected"`
	Restricted bool   `json:"restricted"`
	IsManaged  bool   `json:"is_managed"`
}

type TemplateVar struct {
	Id         string `json:"templatevar_id"`
	Name       string `json:"name"`
	Location   string `json:"location"`
	Local      bool   `json:"local"`
	Protected  bool   `json:"protected"`
	Restricted bool   `json:"restricted"`
	IsManaged  bool   `json:"is_managed"`
}

type TemplateVarList struct {
	Count       int           `json:"count"`
	TemplateVar []TemplateVar `json:"variables"`
}

type TemplateVarCreateResponse struct {
	Status bool   `json:"status"`
	Id     string `json:"templatevar_id"`
}

func (c *ApiClient) GetTemplateVars(ctx context.Context) (*TemplateVarList, int, error) {
	response := &TemplateVarList{}

	code, err := c.httpClient.Get(ctx, "/api/templatevars", response)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}

func (c *ApiClient) UpdateTemplateVar(ctx context.Context, templateVarId string, name string, location string, local bool, value string, protected bool, restricted bool) (int, error) {
	request := TemplateVarValue{
		Name:       name,
		Location:   location,
		Local:      local,
		Value:      value,
		Protected:  protected,
		Restricted: restricted,
	}

	return c.httpClient.Put(ctx, "/api/templatevars/"+templateVarId, request, nil, 200)
}

func (c *ApiClient) CreateTemplateVar(ctx context.Context, name string, location string, local bool, value string, protected bool, restricted bool) (string, int, error) {
	request := TemplateVarValue{
		Name:       name,
		Location:   location,
		Local:      local,
		Value:      value,
		Protected:  protected,
		Restricted: restricted,
	}

	response := &TemplateVarCreateResponse{}

	code, err := c.httpClient.Post(ctx, "/api/templatevars", request, response, 201)
	if err != nil {
		return "", code, err
	}

	return response.Id, code, nil
}

func (c *ApiClient) DeleteTemplateVar(ctx context.Context, templateVarId string) (int, error) {
	return c.httpClient.Delete(ctx, "/api/templatevars/"+templateVarId, nil, nil, 200)
}

func (c *ApiClient) GetTemplateVar(ctx context.Context, templateVarId string) (*TemplateVarValue, int, error) {
	response := &TemplateVarValue{}

	code, err := c.httpClient.Get(ctx, "/api/templatevars/"+templateVarId, response)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}
