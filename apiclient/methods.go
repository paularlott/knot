package apiclient

import (
	"context"
	"errors"
	"fmt"

	"github.com/paularlott/knot/internal/methods"
)

type MethodList struct {
	Count   int                  `json:"count" msgpack:"count"`
	Methods []methods.MethodInfo `json:"methods" msgpack:"methods"`
}

func (c *ApiClient) GetMethods(ctx context.Context) (*MethodList, error) {
	var response MethodList
	statusCode, err := c.httpClient.Get(ctx, "/api/methods", &response)
	if err != nil {
		return nil, err
	}
	if statusCode == 401 {
		return nil, errors.New("unauthorized")
	}
	if statusCode >= 400 {
		return nil, fmt.Errorf("unexpected status code: %d", statusCode)
	}
	return &response, nil
}

func (c *ApiClient) CallMethod(ctx context.Context, request *methods.JSONRPCRequest) (*methods.JSONRPCResponse, error) {
	var response methods.JSONRPCResponse
	statusCode, err := c.httpClient.PostJSON(ctx, "/api/methods/call", request, &response, 200)
	if err != nil && statusCode == 0 {
		return nil, err
	}
	if statusCode == 401 {
		return nil, errors.New("unauthorized")
	}
	if statusCode >= 400 {
		return nil, fmt.Errorf("unexpected status code: %d", statusCode)
	}
	return &response, nil
}

// CallMethodBatch sends a JSON-RPC batch (array of requests) and returns the
// array of responses. Each item in items is sent as-is. The HTTP layer detects
// the array shape on the server side and routes each item independently.
func (c *ApiClient) CallMethodBatch(ctx context.Context, items []methods.JSONRPCRequest) ([]methods.JSONRPCResponse, error) {
	var responses []methods.JSONRPCResponse
	statusCode, err := c.httpClient.PostJSON(ctx, "/api/methods/call", items, &responses, 200)
	if err != nil && statusCode == 0 {
		return nil, err
	}
	if statusCode == 401 {
		return nil, errors.New("unauthorized")
	}
	if statusCode >= 400 {
		return nil, fmt.Errorf("unexpected status code: %d", statusCode)
	}
	return responses, nil
}
