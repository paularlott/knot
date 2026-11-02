package apiclient

import "context"

type TemplateVarValue struct {
	Id         string   `json:"templatevar_id" msgpack:"templatevar_id"`
	Name       string   `json:"name" msgpack:"name"`
	Zones      []string `json:"zones" msgpack:"zones"`
	Local      bool     `json:"local" msgpack:"local"`
	Value      string   `json:"value" msgpack:"value"`
	Protected  bool     `json:"protected" msgpack:"protected"`
	Restricted bool     `json:"restricted" msgpack:"restricted"`
	IsManaged  bool     `json:"is_managed" msgpack:"is_managed"`
}

type TemplateVar struct {
	Id         string   `json:"templatevar_id" msgpack:"templatevar_id"`
	Name       string   `json:"name" msgpack:"name"`
	Zones      []string `json:"zones" msgpack:"zones"`
	Local      bool     `json:"local" msgpack:"local"`
	Protected  bool     `json:"protected" msgpack:"protected"`
	Restricted bool     `json:"restricted" msgpack:"restricted"`
	IsManaged  bool     `json:"is_managed" msgpack:"is_managed"`
}

type TemplateVarList struct {
	Count       int           `json:"count" msgpack:"count"`
	TemplateVar []TemplateVar `json:"variables" msgpack:"variables"`
}

type TemplateVarCreateResponse struct {
	Status bool   `json:"status" msgpack:"status"`
	Id     string `json:"templatevar_id" msgpack:"templatevar_id"`
}

func (c *ApiClient) GetTemplateVars(ctx context.Context) (*TemplateVarList, int, error) {
	response := &TemplateVarList{}

	code, err := c.httpClient.Get(ctx, "/api/templatevars", response)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}

func (c *ApiClient) UpdateTemplateVar(ctx context.Context, templateVarId string, request *TemplateVarValue) (int, error) {
	return c.httpClient.Put(ctx, "/api/templatevars/"+templateVarId, request, nil, 200)
}

func (c *ApiClient) CreateTemplateVar(ctx context.Context, request *TemplateVarValue) (string, int, error) {
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
