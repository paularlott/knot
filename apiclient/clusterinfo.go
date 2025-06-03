package apiclient

import (
	"context"
	"fmt"
)

type ClusterNodeInfo struct {
	Id       string            `json:"id"`
	Address  string            `json:"address"`
	State    string            `json:"state"`
	Metadata map[string]string `json:"metadata"`
}

func (c *ApiClient) GetClusterInfo(ctx context.Context) (*[]ClusterNodeInfo, int, error) {
	response := &[]ClusterNodeInfo{}

	code, err := c.httpClient.Get(ctx, fmt.Sprintf("/api/cluster-info"), response)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}
