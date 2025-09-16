package apiclient

import (
	"context"
	"time"
)

type TokenInfo struct {
	Id           string    `json:"token_id"`
	Name         string    `json:"name"`
	ExpiresAfter time.Time `json:"expires_after"`
}

type CreateTokenRequest struct {
	Name string `json:"name"`
}

type CreateTokenResponse struct {
	Status  bool   `json:"status"`
	TokenID string `json:"token_id"`
}

func (c *ApiClient) GetTokens(ctx context.Context) (*[]TokenInfo, int, error) {
	response := &[]TokenInfo{}

	code, err := c.httpClient.Get(ctx, "/api/tokens", response)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}

func (c *ApiClient) DeleteToken(ctx context.Context, tokenId string) (int, error) {
	return c.httpClient.Delete(ctx, "/api/tokens/"+tokenId, nil, nil, 200)
}

func (c *ApiClient) CreateToken(ctx context.Context, name string) (string, int, error) {
	request := &CreateTokenRequest{
		Name: name,
	}

	response := &CreateTokenResponse{}

	code, err := c.httpClient.Post(ctx, "/api/tokens", request, response, 201)
	if err != nil {
		return "", code, err
	}

	return response.TokenID, code, nil
}
