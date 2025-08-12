package apiclient

import (
	"context"
	"time"

	"github.com/paularlott/knot/internal/database/model"
)

type SpaceRequest struct {
	Name         string             `json:"name"`
	Description  string             `json:"description"`
	TemplateId   string             `json:"template_id"`
	Shell        string             `json:"shell"`
	UserId       string             `json:"user_id"`
	AltNames     []string           `json:"alt_names"`
	IconURL      string             `json:"icon_url"`
	CustomFields []CustomFieldValue `json:"custom_fields"`
}

type CreateSpaceResponse struct {
	Status  bool   `json:"status"`
	SpaceID string `json:"space_id"`
}

type SpaceTransferRequest struct {
	UserId string `json:"user_id"`
}

type SpaceInfo struct {
	Id              string            `json:"space_id"`
	Name            string            `json:"name"`
	Description     string            `json:"description"`
	Note            string            `json:"note"`
	TemplateName    string            `json:"template_name"`
	TemplateId      string            `json:"template_id"`
	Zone            string            `json:"zone"`
	Username        string            `json:"username"`
	UserId          string            `json:"user_id"`
	Platform        string            `json:"platform"`
	SharedUserId    string            `json:"shared_user_id"`
	SharedUsername  string            `json:"shared_username"`
	HasCodeServer   bool              `json:"has_code_server"`
	HasSSH          bool              `json:"has_ssh"`
	HasHttpVNC      bool              `json:"has_http_vnc"`
	HasTerminal     bool              `json:"has_terminal"`
	HasState        bool              `json:"has_state"`
	IsDeployed      bool              `json:"is_deployed"`
	IsPending       bool              `json:"is_pending"`
	IsDeleting      bool              `json:"is_deleting"`
	TcpPorts        map[string]string `json:"tcp_ports"`
	HttpPorts       map[string]string `json:"http_ports"`
	UpdateAvailable bool              `json:"update_available"`
	IsRemote        bool              `json:"is_remote"`
	HasVSCodeTunnel bool              `json:"has_vscode_tunnel"`
	VSCodeTunnel    string            `json:"vscode_tunnel_name"`
	StartedAt       time.Time         `json:"started_at"`
	IconURL         string            `json:"icon_url"`
}

type SpaceInfoList struct {
	Count  int         `json:"count"`
	Spaces []SpaceInfo `json:"spaces"`
}

type CustomFieldValue struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type SpaceDefinition struct {
	UserId       string                       `json:"user_id"`
	TemplateId   string                       `json:"template_id"`
	Name         string                       `json:"name"`
	Description  string                       `json:"description"`
	Shell        string                       `json:"shell"`
	Zone         string                       `json:"zone"`
	AltNames     []string                     `json:"alt_names"`
	IsDeployed   bool                         `json:"is_deployed"`
	IsPending    bool                         `json:"is_pending"`
	IsDeleting   bool                         `json:"is_deleting"`
	VolumeData   map[string]model.SpaceVolume `json:"volume_data"`
	StartedAt    time.Time                    `json:"started_at"`
	IconURL      string                       `json:"icon_url"`
	CustomFields []CustomFieldValue           `json:"custom_fields"`
}

type RunCommandRequest struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	Timeout int      `json:"timeout"`
	Workdir string   `json:"workdir,omitempty"`
}

func (c *ApiClient) GetSpaces(ctx context.Context, userId string) (*SpaceInfoList, int, error) {
	response := &SpaceInfoList{}

	code, err := c.httpClient.Get(ctx, "/api/spaces?user_id="+userId, &response)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}

func (c *ApiClient) GetSpace(ctx context.Context, spaceId string) (*SpaceDefinition, int, error) {
	response := &SpaceDefinition{}

	code, err := c.httpClient.Get(ctx, "/api/spaces/"+spaceId, &response)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}

func (c *ApiClient) UpdateSpace(ctx context.Context, spaceId string, space *SpaceRequest) (int, error) {
	code, err := c.httpClient.Put(ctx, "/api/spaces/"+spaceId, space, nil, 200)
	if err != nil {
		return code, err
	}

	return code, nil
}

func (c *ApiClient) CreateSpace(ctx context.Context, space *SpaceRequest) (string, int, error) {
	response := &CreateSpaceResponse{}

	code, err := c.httpClient.Post(ctx, "/api/spaces", space, response, 201)
	if err != nil {
		return "", code, err
	}

	return response.SpaceID, code, nil
}

func (c *ApiClient) DeleteSpace(ctx context.Context, spaceId string) (int, error) {
	return c.httpClient.Delete(ctx, "/api/spaces/"+spaceId, nil, nil, 200)
}

func (c *ApiClient) StartSpace(ctx context.Context, spaceId string) (int, error) {
	return c.httpClient.Post(ctx, "/api/spaces/"+spaceId+"/start", nil, nil, 200)
}

func (c *ApiClient) StopSpace(ctx context.Context, spaceId string) (int, error) {
	return c.httpClient.Post(ctx, "/api/spaces/"+spaceId+"/stop", nil, nil, 200)
}

func (c *ApiClient) RestartSpace(ctx context.Context, spaceId string) (int, error) {
	return c.httpClient.Post(ctx, "/api/spaces/"+spaceId+"/restart", nil, nil, 200)
}

func (c *ApiClient) TransferSpace(ctx context.Context, spaceId string, userId string) (int, error) {
	request := &SpaceTransferRequest{
		UserId: userId,
	}

	return c.httpClient.Post(ctx, "/api/spaces/"+spaceId+"/transfer", request, nil, 200)
}
