package apiclient

import (
	"context"
)

type VolumeInfo struct {
	Id           string `json:"volume_id" msgpack:"volume_id"`
	Name         string `json:"name" msgpack:"name"`
	Active       bool   `json:"active" msgpack:"active"`
	Zone         string `json:"zone" msgpack:"zone"`
	NodeId       string `json:"node_id" msgpack:"node_id"`
	NodeHostname string `json:"node_hostname" msgpack:"node_hostname"`
	Platform     string `json:"platform" msgpack:"platform"`
}

type VolumeInfoList struct {
	Count   int          `json:"count" msgpack:"count"`
	Volumes []VolumeInfo `json:"volumes" msgpack:"volumes"`
}

type VolumeDefinition struct {
	VolumeId     string `json:"volume_id" msgpack:"volume_id"`
	Name         string `json:"name" msgpack:"name"`
	Definition   string `json:"definition" msgpack:"definition"`
	Zone         string `json:"zone" msgpack:"zone"`
	NodeId       string `json:"node_id" msgpack:"node_id"`
	NodeHostname string `json:"node_hostname" msgpack:"node_hostname"`
	Active       bool   `json:"active" msgpack:"active"`
	Platform     string `json:"platform" msgpack:"platform"`
}

type VolumeUpdateRequest struct {
	Name       string `json:"name" msgpack:"name"`
	Definition string `json:"definition" msgpack:"definition"`
	Platform   string `json:"platform" msgpack:"platform"`
	NodeId     string `json:"node_id" msgpack:"node_id"`
}

type VolumeCreateRequest struct {
	Name       string `json:"name" msgpack:"name"`
	Definition string `json:"definition" msgpack:"definition"`
	Platform   string `json:"platform" msgpack:"platform"`
	NodeId     string `json:"node_id" msgpack:"node_id"`
}

type VolumeCreateResponse struct {
	Status   bool   `json:"status" msgpack:"status"`
	VolumeId string `json:"volume_id" msgpack:"volume_id"`
}

type VolumeStartStopRequest struct {
	Zone string `json:"zone" msgpack:"zone"`
}
type VolumeStartRequest = VolumeStartStopRequest
type VolumeStopRequest = VolumeStartStopRequest

type VolumeStartStopResponse struct {
	Name       string                 `json:"name" msgpack:"name"`
	Definition string                 `json:"definition" msgpack:"definition"`
	Zone       string                 `json:"zone" msgpack:"zone"`
	Variables  map[string]interface{} `json:"variables" msgpack:"variables"`
}
type VolumeStartResponse = VolumeStartStopResponse
type VolumeStopResponse = VolumeStartStopResponse

type StartVolumeResponse struct {
	Status bool   `json:"status" msgpack:"status"`
	Zone   string `json:"zone" msgpack:"zone"`
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

type VolumeNode struct {
	NodeId   string `json:"node_id" msgpack:"node_id"`
	Hostname string `json:"hostname" msgpack:"hostname"`
}

func (c *ApiClient) GetVolumeNodes(ctx context.Context, platform string) ([]VolumeNode, int, error) {
	var response []VolumeNode

	code, err := c.httpClient.Get(ctx, "/api/volumes/nodes?platform="+platform, &response)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
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
