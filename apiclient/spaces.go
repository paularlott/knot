package apiclient

import (
	"time"

	"github.com/paularlott/knot/database/model"
)

type SpaceRequest struct {
	Name        string           `json:"name"`
	TemplateId  string           `json:"template_id"`
	AgentURL    string           `json:"agent_url"`
	Shell       string           `json:"shell"`
	UserId      string           `json:"user_id"`
	VolumeSizes map[string]int64 `json:"volume_sizes"`
	AltNames    []string         `json:"alt_names"`
	IsDeployed  bool             `json:"is_deployed"`
	Location    string           `json:"location"`
}
type CreateSpaceRequest = SpaceRequest
type UpdateSpaceRequest = SpaceRequest

type CreateSpaceResponse struct {
	Status  bool   `json:"status"`
	SpaceID string `json:"space_id"`
}

type SpaceInfo struct {
	Id           string `json:"space_id"`
	Name         string `json:"name"`
	TemplateName string `json:"template_name"`
	TemplateId   string `json:"template_id"`
	Location     string `json:"location"`
	Username     string `json:"username"`
	UserId       string `json:"user_id"`
	VolumeSize   int    `json:"volume_size"`
}

type SpaceServiceState struct {
	Name            string `json:"name"`
	Location        string `json:"location"`
	HasCodeServer   bool   `json:"has_code_server"`
	HasSSH          bool   `json:"has_ssh"`
	HasHttpVNC      bool   `json:"has_http_vnc"`
	HasTerminal     bool   `json:"has_terminal"`
	IsDeployed      bool   `json:"is_deployed"`
	TcpPorts        []int  `json:"tcp_ports"`
	HttpPorts       []int  `json:"http_ports"`
	UpdateAvailable bool   `json:"update_available"`
}

type SpaceDefinition struct {
	UserId      string                       `json:"user_id"`
	TemplateId  string                       `json:"template_id"`
	Name        string                       `json:"name"`
	AgentURL    string                       `json:"agent_url"`
	Shell       string                       `json:"shell"`
	Location    string                       `json:"location"`
	AltNames    []string                     `json:"alt_names"`
	IsDeployed  bool                         `json:"is_deployed"`
	VolumeSizes map[string]int64             `json:"volume_sizes"`
	VolumeData  map[string]model.SpaceVolume `json:"volume_data"`
}

func (c *ApiClient) GetSpaces(userId string) ([]*SpaceInfo, int, error) {
	response := []*SpaceInfo{}

	code, err := c.httpClient.Get("/api/v1/spaces?user_id="+userId, &response)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}

func (c *ApiClient) GetSpaceServiceState(spaceId string) (*SpaceServiceState, int, error) {
	response := &SpaceServiceState{}

	code, err := c.httpClient.Get("/api/v1/spaces/"+spaceId+"/service_state", &response)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}

func (c *ApiClient) GetSpace(spaceId string) (*model.Space, int, error) {
	response := &SpaceDefinition{}

	code, err := c.httpClient.Get("/api/v1/spaces/"+spaceId, &response)
	if err != nil {
		return nil, code, err
	}

	space := &model.Space{
		Id:           spaceId,
		UserId:       response.UserId,
		TemplateId:   response.TemplateId,
		Name:         response.Name,
		AltNames:     response.AltNames,
		AgentURL:     response.AgentURL,
		Shell:        response.Shell,
		TemplateHash: "",
		IsDeployed:   response.IsDeployed,
		VolumeData:   response.VolumeData,
		VolumeSizes:  response.VolumeSizes,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
		Location:     response.Location,
	}

	return space, code, nil
}

func (c *ApiClient) UpdateSpace(space *model.Space) (int, error) {
	request := &UpdateSpaceRequest{
		UserId:      space.UserId,
		TemplateId:  space.TemplateId,
		Name:        space.Name,
		AltNames:    space.AltNames,
		AgentURL:    space.AgentURL,
		Shell:       space.Shell,
		VolumeSizes: space.VolumeSizes,
		IsDeployed:  space.IsDeployed,
		Location:    space.Location,
	}

	code, err := c.httpClient.Put("/api/v1/spaces/"+space.Id, request, nil, 200)
	if err != nil {
		return code, err
	}

	return code, nil
}

func (c *ApiClient) CreateSpace(space *model.Space) (int, error) {
	request := &CreateSpaceRequest{
		UserId:      space.UserId,
		TemplateId:  space.TemplateId,
		Name:        space.Name,
		AltNames:    space.AltNames,
		AgentURL:    space.AgentURL,
		Shell:       space.Shell,
		VolumeSizes: space.VolumeSizes,
	}

	response := &CreateSpaceResponse{}

	code, err := c.httpClient.Post("/api/v1/spaces", request, response, 201)
	if err != nil {
		return code, err
	}

	// Match ID to core server
	space.Id = response.SpaceID

	return code, nil
}

func (c *ApiClient) DeleteSpace(spaceId string) (int, error) {
	return c.httpClient.Delete("/api/v1/spaces/"+spaceId, nil, nil, 200)
}
