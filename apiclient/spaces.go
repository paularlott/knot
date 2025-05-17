package apiclient

import (
	"time"

	"github.com/paularlott/knot/internal/database/model"
)

type SpaceRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	TemplateId  string   `json:"template_id"`
	Shell       string   `json:"shell"`
	UserId      string   `json:"user_id"`
	AltNames    []string `json:"alt_names"`
	Location    string   `json:"location"`
}
type CreateSpaceRequest = SpaceRequest
type UpdateSpaceRequest = SpaceRequest

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
	Location        string            `json:"location"`
	Username        string            `json:"username"`
	UserId          string            `json:"user_id"`
	LocalContainer  bool              `json:"local_container"`
	IsManual        bool              `json:"is_manual"`
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
}

type SpaceInfoList struct {
	Count  int         `json:"count"`
	Spaces []SpaceInfo `json:"spaces"`
}

type SpaceDefinition struct {
	UserId      string                       `json:"user_id"`
	TemplateId  string                       `json:"template_id"`
	Name        string                       `json:"name"`
	Description string                       `json:"description"`
	Shell       string                       `json:"shell"`
	Location    string                       `json:"location"`
	AltNames    []string                     `json:"alt_names"`
	IsDeployed  bool                         `json:"is_deployed"`
	IsPending   bool                         `json:"is_pending"`
	IsDeleting  bool                         `json:"is_deleting"`
	VolumeData  map[string]model.SpaceVolume `json:"volume_data"`
}

func (c *ApiClient) GetSpaces(userId string) (*SpaceInfoList, int, error) {
	response := &SpaceInfoList{}

	code, err := c.httpClient.Get("/api/spaces?user_id="+userId, &response)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}

func (c *ApiClient) GetSpace(spaceId string) (*model.Space, int, error) {
	response := &SpaceDefinition{}

	code, err := c.httpClient.Get("/api/spaces/"+spaceId, &response)
	if err != nil {
		return nil, code, err
	}

	now := time.Now().UTC()

	space := &model.Space{
		Id:           spaceId,
		UserId:       response.UserId,
		TemplateId:   response.TemplateId,
		Name:         response.Name,
		Description:  response.Description,
		AltNames:     response.AltNames,
		Shell:        response.Shell,
		TemplateHash: "",
		IsDeployed:   response.IsDeployed,
		IsPending:    response.IsPending,
		IsDeleting:   response.IsDeleting,
		VolumeData:   response.VolumeData,
		CreatedAt:    now,
		UpdatedAt:    now,
		Location:     response.Location,
	}

	return space, code, nil
}

func (c *ApiClient) UpdateSpace(space *model.Space) (int, error) {
	request := &UpdateSpaceRequest{
		UserId:     space.UserId,
		TemplateId: space.TemplateId,
		Name:       space.Name,
		AltNames:   space.AltNames,
		Shell:      space.Shell,
		Location:   space.Location,
	}

	code, err := c.httpClient.Put("/api/spaces/"+space.Id, request, nil, 200)
	if err != nil {
		return code, err
	}

	return code, nil
}

func (c *ApiClient) CreateSpace(space *model.Space) (int, error) {
	request := &CreateSpaceRequest{
		UserId:     space.UserId,
		TemplateId: space.TemplateId,
		Name:       space.Name,
		AltNames:   space.AltNames,
		Shell:      space.Shell,
		Location:   space.Location,
	}

	response := &CreateSpaceResponse{}

	code, err := c.httpClient.Post("/api/spaces", request, response, 201)
	if err != nil {
		return code, err
	}

	// Match ID to core server
	space.Id = response.SpaceID

	return code, nil
}

func (c *ApiClient) DeleteSpace(spaceId string) (int, error) {
	return c.httpClient.Delete("/api/spaces/"+spaceId, nil, nil, 200)
}

func (c *ApiClient) StartSpace(spaceId string) (int, error) {
	return c.httpClient.Post("/api/spaces/"+spaceId+"/start", nil, nil, 200)
}

func (c *ApiClient) StopSpace(spaceId string) (int, error) {
	return c.httpClient.Post("/api/spaces/"+spaceId+"/stop", nil, nil, 200)
}

func (c *ApiClient) TransferSpace(spaceId string, userId string) (int, error) {
	request := &SpaceTransferRequest{
		UserId: userId,
	}

	return c.httpClient.Post("/api/spaces/"+spaceId+"/transfer", request, nil, 200)
}
