package apiclient

import "context"

type StackDefCustomField struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type StackDefPortForward struct {
	ToSpace    string `json:"to_space"`
	LocalPort  uint16 `json:"local_port"`
	RemotePort uint16 `json:"remote_port"`
}

type StackDefSpace struct {
	Name          string                `json:"name"`
	TemplateId    string                `json:"template_id"`
	Description   string                `json:"description"`
	Shell         string                `json:"shell"`
	StartupScript string                `json:"startup_script_id,omitempty"`
	DependsOn     []string              `json:"depends_on"`
	CustomFields  []StackDefCustomField `json:"custom_fields"`
	PortForwards  []StackDefPortForward `json:"port_forwards"`
}

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Space   string `json:"space,omitempty"`
}

type StackDefinitionValidationResponse struct {
	Valid  bool              `json:"valid"`
	Errors []ValidationError `json:"errors,omitempty"`
}

type StackDefinitionRequest struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Active      bool            `json:"active"`
	Scope       string          `json:"scope"`
	Groups      []string        `json:"groups"`
	Zones       []string        `json:"zones"`
	Spaces      []StackDefSpace `json:"spaces"`
}

type StackDefinitionCreateResponse struct {
	Status bool   `json:"status"`
	Id     string `json:"stack_definition_id"`
}

type StackDefinitionInfo struct {
	Id          string          `json:"stack_definition_id"`
	UserId      string          `json:"user_id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Active      bool            `json:"active"`
	Scope       string          `json:"scope"`
	Groups      []string        `json:"groups"`
	Zones       []string        `json:"zones"`
	Spaces      []StackDefSpace `json:"spaces"`
	IsManaged   bool            `json:"is_managed"`
}

type StackDefinitionList struct {
	Count       int                   `json:"count"`
	Definitions []StackDefinitionInfo `json:"stack_definitions"`
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
