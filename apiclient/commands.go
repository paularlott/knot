package apiclient

import "context"

type CommandList struct {
	Count    int           `json:"count"`
	Commands []CommandInfo `json:"commands"`
}

type CommandInfo struct {
	Id           string   `json:"command_id"`
	UserId       string   `json:"user_id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	ArgumentHint string   `json:"argument_hint"`
	AllowedTools []string `json:"allowed_tools"`
	Groups       []string `json:"groups"`
	Zones        []string `json:"zones"`
	Active       bool     `json:"active"`
	IsManaged    bool     `json:"is_managed"`
}

type CommandDetails struct {
	Id           string   `json:"command_id"`
	UserId       string   `json:"user_id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	ArgumentHint string   `json:"argument_hint"`
	AllowedTools []string `json:"allowed_tools"`
	Body         string   `json:"body"`
	Groups       []string `json:"groups"`
	Zones        []string `json:"zones"`
	Active       bool     `json:"active"`
	IsManaged    bool     `json:"is_managed"`
}

type CommandCreateRequest struct {
	UserId       string   `json:"user_id"`
	Content      string   `json:"content"`
	Groups       []string `json:"groups"`
	Zones        []string `json:"zones"`
	Active       bool     `json:"active"`
}

type CommandUpdateRequest struct {
	Content string   `json:"content"`
	Groups  []string `json:"groups"`
	Zones   []string `json:"zones"`
	Active  bool     `json:"active"`
}

type CommandCreateResponse struct {
	Status bool   `json:"status"`
	Id     string `json:"command_id"`
}

type CommandUpdateResponse struct {
	Status bool `json:"status"`
}

func (c *ApiClient) GetCommands(ctx context.Context) (*CommandList, error) {
	var commands CommandList
	_, err := c.httpClient.Get(ctx, "/api/command", &commands)
	return &commands, err
}

func (c *ApiClient) GetCommand(ctx context.Context, nameOrId string) (*CommandDetails, error) {
	var command CommandDetails
	_, err := c.httpClient.Get(ctx, "/api/command/"+nameOrId, &command)
	return &command, err
}

func (c *ApiClient) CreateCommand(ctx context.Context, req *CommandCreateRequest) (*CommandCreateResponse, error) {
	var resp CommandCreateResponse
	_, err := c.httpClient.Post(ctx, "/api/command", req, &resp, 201)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *ApiClient) UpdateCommand(ctx context.Context, nameOrId string, req *CommandUpdateRequest) (*CommandUpdateResponse, error) {
	var resp CommandUpdateResponse
	_, err := c.httpClient.Put(ctx, "/api/command/"+nameOrId, req, &resp, 200)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *ApiClient) DeleteCommand(ctx context.Context, nameOrId string) error {
	_, err := c.httpClient.Delete(ctx, "/api/command/"+nameOrId, nil, nil, 0)
	return err
}
