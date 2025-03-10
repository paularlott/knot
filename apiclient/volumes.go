package apiclient

import "github.com/paularlott/knot/database/model"

type VolumeInfo struct {
	Id             string `json:"volume_id"`
	Name           string `json:"name"`
	Active         bool   `json:"active"`
	Location       string `json:"location"`
	LocalContainer bool   `json:"local_container"`
}

type VolumeInfoList struct {
	Count   int          `json:"count"`
	Volumes []VolumeInfo `json:"volumes"`
}

type VolumeDefinition struct {
	Name           string `json:"name"`
	Definition     string `json:"definition"`
	Location       string `json:"location"`
	Active         bool   `json:"active"`
	LocalContainer bool   `json:"local_container"`
}

type VolumeUpdateRequest struct {
	Name       string `json:"name"`
	Definition string `json:"definition"`
}

type VolumeCreateRequest struct {
	Name           string `json:"name"`
	Definition     string `json:"definition"`
	LocalContainer bool   `json:"local_container"`
}

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

func (c *ApiClient) GetVolumes() (*VolumeInfoList, int, error) {
	response := &VolumeInfoList{}

	code, err := c.httpClient.Get("/api/volumes", response)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}

func (c *ApiClient) CreateVolume(name string, definition string, localContainer bool) (*VolumeCreateResponse, int, error) {
	request := VolumeCreateRequest{
		Name:           name,
		Definition:     definition,
		LocalContainer: localContainer,
	}

	response := &VolumeCreateResponse{}

	code, err := c.httpClient.Post("/api/volumes", request, response, 201)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}

func (c *ApiClient) UpdateVolume(volumeId string, name string, definition string) (int, error) {
	request := VolumeUpdateRequest{
		Name:       name,
		Definition: definition,
	}

	return c.httpClient.Put("/api/volumes/"+volumeId, request, nil, 200)
}

func (c *ApiClient) DeleteVolume(volumeId string) (int, error) {
	return c.httpClient.Delete("/api/volumes/"+volumeId, nil, nil, 200)
}

func (c *ApiClient) GetVolume(volumeId string) (*VolumeDefinition, int, error) {
	response := VolumeDefinition{}

	code, err := c.httpClient.Get("/api/volumes/"+volumeId, &response)
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
		Id:             volumeId,
		Name:           response.Name,
		Definition:     response.Definition,
		Location:       response.Location,
		Active:         response.Active,
		LocalContainer: response.LocalContainer,
	}

	return volume, code, nil
}

func (c *ApiClient) StartVolume(volumeId string) (*StartVolumeResponse, int, error) {
	response := StartVolumeResponse{}

	code, err := c.httpClient.Post("/api/volumes/"+volumeId+"/start", nil, &response, 200)
	if err != nil {
		return nil, code, err
	}

	return &response, code, nil
}

func (c *ApiClient) StopVolume(volumeId string) (int, error) {
	return c.httpClient.Post("/api/volumes/"+volumeId+"/stop", nil, nil, 200)
}
