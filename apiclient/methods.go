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
		// Transport-level failure (no HTTP response received).
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
