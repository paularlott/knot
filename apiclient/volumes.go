package apiclient

import "github.com/paularlott/knot/database/model"

type VolumeInfo struct {
	Id       string `json:"volume_id"`
	Name     string `json:"name"`
	Active   bool   `json:"active"`
	Location string `json:"location"`
}

type VolumeDefinition struct {
	Name       string `json:"name"`
	Definition string `json:"definition"`
	Location   string `json:"location"`
	Active     bool   `json:"active"`
}

type VolumeRequest struct {
	Name       string `json:"name"`
	Definition string `json:"definition"`
}
type CreateVolumeRequest = VolumeRequest
type UpdateVolumeRequest = VolumeRequest

type VolumeCreateResponse struct {
	Status   bool   `json:"status"`
	VolumeId string `json:"volume_id"`
}

type VolumeStartStopRequest struct {
	Location string `json:"location"`
}
type VolumeStartRequest = VolumeStartStopRequest
type VolumeStopRequest = VolumeStartStopRequest

type VolumeStartStopResponse struct {
	Name       string                 `json:"name"`
	Definition string                 `json:"definition"`
	Location   string                 `json:"location"`
	Variables  map[string]interface{} `json:"variables"`
}
type VolumeStartResponse = VolumeStartStopResponse
type VolumeStopResponse = VolumeStartStopResponse

type StartVolumeResponse struct {
	Status   bool   `json:"status"`
	Location string `json:"location"`
}

func (c *ApiClient) GetVolumes() (*[]VolumeInfo, int, error) {
	response := &[]VolumeInfo{}

	code, err := c.httpClient.Get("/api/v1/volumes", response)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}

func (c *ApiClient) CreateVolume(name string, definition string) (*VolumeCreateResponse, int, error) {
	request := VolumeRequest{
		Name:       name,
		Definition: definition,
	}

	response := &VolumeCreateResponse{}

	code, err := c.httpClient.Post("/api/v1/volumes", request, response, 201)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}

func (c *ApiClient) UpdateVolume(volumeId string, name string, definition string) (int, error) {
	request := VolumeRequest{
		Name:       name,
		Definition: definition,
	}

	return c.httpClient.Put("/api/v1/volumes/"+volumeId, request, nil, 200)
}

func (c *ApiClient) DeleteVolume(volumeId string) (int, error) {
	return c.httpClient.Delete("/api/v1/volumes/"+volumeId, nil, nil, 200)
}

func (c *ApiClient) GetVolume(volumeId string) (*VolumeDefinition, int, error) {
	response := VolumeDefinition{}

	code, err := c.httpClient.Get("/api/v1/volumes/"+volumeId, &response)
	if err != nil {
		return nil, code, err
	}

	return &response, code, nil
}

func (c *ApiClient) GetVolumeObject(volumeId string) (*model.Volume, int, error) {
	response, code, err := c.GetVolume(volumeId)
	if err != nil {
		return nil, code, err
	}

	volume := &model.Volume{
		Id:         volumeId,
		Name:       response.Name,
		Definition: response.Definition,
		Location:   response.Location,
		Active:     response.Active,
	}

	return volume, code, nil
}

func (c *ApiClient) StartVolume(volumeId string) (*StartVolumeResponse, int, error) {
	response := StartVolumeResponse{}

	code, err := c.httpClient.Post("/api/v1/volumes/"+volumeId+"/start", nil, &response, 200)
	if err != nil {
		return nil, code, err
	}

	return &response, code, nil
}

func (c *ApiClient) StopVolume(volumeId string) (int, error) {
	return c.httpClient.Post("/api/v1/volumes/"+volumeId+"/stop", nil, nil, 200)
}
