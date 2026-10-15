package apiclient

import (
	"context"
	"fmt"
)

type ClusterNodeInfo struct {
	Id       string            `json:"id" msgpack:"id"`
	Address  string            `json:"address" msgpack:"address"`
	State    string            `json:"state" msgpack:"state"`
	Metadata map[string]string `json:"metadata" msgpack:"metadata"`
}

type ClusterNode struct {
	NodeId          string   `json:"node_id" msgpack:"node_id"`
	Hostname        string   `json:"hostname" msgpack:"hostname"`
	Zone            string   `json:"zone" msgpack:"zone"`
	ApiEndpoint     string   `json:"api_endpoint" msgpack:"api_endpoint"`
	AllocatedSpaces int      `json:"allocated_spaces" msgpack:"allocated_spaces"`
	RunningSpaces   int      `json:"running_spaces" msgpack:"running_spaces"`
	Runtimes        []string `json:"runtimes" msgpack:"runtimes"`
}

func (c *ApiClient) GetClusterInfo(ctx context.Context) (*[]ClusterNodeInfo, int, error) {
	response := &[]ClusterNodeInfo{}

	code, err := c.httpClient.Get(ctx, fmt.Sprintf("/api/cluster-info"), response)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}

func (c *ApiClient) GetClusterNodes(ctx context.Context) (*[]ClusterNode, int, error) {
	response := &[]ClusterNode{}

	code, err := c.httpClient.Get(ctx, fmt.Sprintf("/api/cluster/nodes"), response)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}
