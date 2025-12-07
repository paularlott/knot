package apiclient

import (
	"context"
)

type VolumeInfo struct {
	Id       string `json:"volume_id"`
	Name     string `json:"name"`
	Active   bool   `json:"active"`
	Zone     string `json:"zone"`
	Platform string `json:"platform"`
}

type VolumeInfoList struct {
	Count   int          `json:"count"`
	Volumes []VolumeInfo `json:"volumes"`
}

type VolumeDefinition struct {
	VolumeId   string `json:"volume_id"`
	Name       string `json:"name"`
	Definition string `json:"definition"`
	Zone       string `json:"zone"`
	Active     bool   `json:"active"`
	Platform   string `json:"platform"`
}

type VolumeUpdateRequest struct {
	Name       string `json:"name"`
	Definition string `json:"definition"`
	Platform   string `json:"platform"`
}

type VolumeCreateRequest struct {
	Name       string `json:"name"`
	Definition string `json:"definition"`
	Platform   string `json:"platform"`
}

type VolumeCreateResponse struct {
	Status   bool   `json:"status"`
	VolumeId string `json:"volume_id"`
}

type VolumeStartStopRequest struct {
	Zone string `json:"zone"`
}
type VolumeStartRequest = VolumeStartStopRequest
type VolumeStopRequest = VolumeStartStopRequest

type VolumeStartStopResponse struct {
	Name       string                 `json:"name"`
	Definition string                 `json:"definition"`
	Zone       string                 `json:"zone"`
	Variables  map[string]interface{} `json:"variables"`
}
type VolumeStartResponse = VolumeStartStopResponse
type VolumeStopResponse = VolumeStartStopResponse

type StartVolumeResponse struct {
	Status bool   `json:"status"`
	Zone   string `json:"zone"`
}

func (c *ApiClient) GetVolumes(ctx context.Context) (*VolumeInfoList, int, error) {
	response := &VolumeInfoList{}

	code, err := c.httpClient.Get(ctx, "/api/volumes", response)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}

func (c *ApiClient) CreateVolume(ctx context.Context, request *VolumeCreateRequest) (*VolumeCreateResponse, int, error) {
	response := &VolumeCreateResponse{}

	code, err := c.httpClient.Post(ctx, "/api/volumes", request, response, 201)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}

func (c *ApiClient) UpdateVolume(ctx context.Context, volumeId string, request *VolumeUpdateRequest) (int, error) {
	return c.httpClient.Put(ctx, "/api/volumes/"+volumeId, request, nil, 200)
}

func (c *ApiClient) DeleteVolume(ctx context.Context, volumeId string) (int, error) {
	return c.httpClient.Delete(ctx, "/api/volumes/"+volumeId, nil, nil, 200)
}

func (c *ApiClient) GetVolume(ctx context.Context, volumeId string) (*VolumeDefinition, int, error) {
	response := VolumeDefinition{}

	code, err := c.httpClient.Get(ctx, "/api/volumes/"+volumeId, &response)
	if err != nil {
		return nil, code, err
	}

	return &response, code, nil
}

func (c *ApiClient) StartVolume(ctx context.Context, volumeId string) (*StartVolumeResponse, int, error) {
	response := StartVolumeResponse{}

	code, err := c.httpClient.Post(ctx, "/api/volumes/"+volumeId+"/start", nil, &response, 200)
	if err != nil {
		return nil, code, err
	}

	return &response, code, nil
}

func (c *ApiClient) StopVolume(ctx context.Context, volumeId string) (int, error) {
	return c.httpClient.Post(ctx, "/api/volumes/"+volumeId+"/stop", nil, nil, 200)
}
