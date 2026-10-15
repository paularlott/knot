package apiclient

import "context"

type StackDefCustomField struct {
	Name  string `json:"name" msgpack:"name"`
	Value string `json:"value" msgpack:"value"`
}

type StackDefPortForward struct {
	ToSpace    string `json:"to_space" msgpack:"to_space"`
	LocalPort  uint16 `json:"local_port" msgpack:"local_port"`
	RemotePort uint16 `json:"remote_port" msgpack:"remote_port"`
}

type StackDefSpace struct {
	Name          string                `json:"name" msgpack:"name"`
	TemplateId    string                `json:"template_id" msgpack:"template_id"`
	Description   string                `json:"description" msgpack:"description"`
	Shell         string                `json:"shell" msgpack:"shell"`
	StartupScript string                `json:"startup_script_id,omitempty" msgpack:"startup_script_id,omitempty"`
	DependsOn     []string              `json:"depends_on" msgpack:"depends_on"`
	CustomFields  []StackDefCustomField `json:"custom_fields" msgpack:"custom_fields"`
	PortForwards  []StackDefPortForward `json:"port_forwards" msgpack:"port_forwards"`
}

type ValidationError struct {
	Field   string `json:"field" msgpack:"field"`
	Message string `json:"message" msgpack:"message"`
	Line    int    `json:"line,omitempty" msgpack:"line,omitempty"`
	Space   string `json:"space,omitempty" msgpack:"space,omitempty"`
}

type StackDefinitionValidationResponse struct {
	Valid  bool              `json:"valid" msgpack:"valid"`
	Errors []ValidationError `json:"errors,omitempty" msgpack:"errors,omitempty"`
}

type StackDefinitionRequest struct {
	Name        string          `json:"name" msgpack:"name"`
	Description string          `json:"description" msgpack:"description"`
	IconURL     string          `json:"icon_url" msgpack:"icon_url"`
	Active      bool            `json:"active" msgpack:"active"`
	Scope       string          `json:"scope" msgpack:"scope"`
	Groups      []string        `json:"groups" msgpack:"groups"`
	Zones       []string        `json:"zones" msgpack:"zones"`
	Spaces      []StackDefSpace `json:"spaces" msgpack:"spaces"`
}

type StackDefinitionCreateResponse struct {
	Status bool   `json:"status" msgpack:"status"`
	Id     string `json:"stack_definition_id" msgpack:"stack_definition_id"`
}

type StackDefinitionInfo struct {
	Id          string          `json:"stack_definition_id" msgpack:"stack_definition_id"`
	UserId      string          `json:"user_id" msgpack:"user_id"`
	Name        string          `json:"name" msgpack:"name"`
	Description string          `json:"description" msgpack:"description"`
	IconURL     string          `json:"icon_url" msgpack:"icon_url"`
	Active      bool            `json:"active" msgpack:"active"`
	Scope       string          `json:"scope" msgpack:"scope"`
	Groups      []string        `json:"groups" msgpack:"groups"`
	Zones       []string        `json:"zones" msgpack:"zones"`
	Spaces      []StackDefSpace `json:"spaces" msgpack:"spaces"`
	IsManaged   bool            `json:"is_managed" msgpack:"is_managed"`
}

type StackDefinitionList struct {
	Count       int                   `json:"count" msgpack:"count"`
	Definitions []StackDefinitionInfo `json:"stack_definitions" msgpack:"stack_definitions"`
}

func (c *ApiClient) GetStackDefinitions(ctx context.Context) (*StackDefinitionList, int, error) {
	response := &StackDefinitionList{}
	code, err := c.httpClient.Get(ctx, "/api/stack-definitions", response)
	if err != nil {
		return nil, code, err
	}
	return response, code, nil
}

func (c *ApiClient) GetStackDefinitionByName(ctx context.Context, name string) (*StackDefinitionInfo, error) {
	list, _, err := c.GetStackDefinitions(ctx)
	if err != nil {
		return nil, err
	}
	for i := range list.Definitions {
		if list.Definitions[i].Name == name {
			return &list.Definitions[i], nil
		}
	}
	return nil, nil
}

func (c *ApiClient) CreateStackDefinition(ctx context.Context, req *StackDefinitionRequest) (string, int, error) {
	response := &StackDefinitionCreateResponse{}
	code, err := c.httpClient.Post(ctx, "/api/stack-definitions", req, response, 201)
	if err != nil {
		return "", code, err
	}
	return response.Id, code, nil
}

func (c *ApiClient) UpdateStackDefinition(ctx context.Context, id string, req *StackDefinitionRequest) (int, error) {
	return c.httpClient.Put(ctx, "/api/stack-definitions/"+id, req, nil, 200)
}

func (c *ApiClient) ValidateStackDefinition(ctx context.Context, req *StackDefinitionRequest) (*StackDefinitionValidationResponse, int, error) {
	response := &StackDefinitionValidationResponse{}
	code, err := c.httpClient.Post(ctx, "/api/stack-definitions/validate", req, response, 200)
	if err != nil {
		return nil, code, err
	}
	return response, code, nil
}

func (c *ApiClient) DeleteStackDefinition(ctx context.Context, id string) (int, error) {
	return c.httpClient.Delete(ctx, "/api/stack-definitions/"+id, nil, nil, 200)
}
