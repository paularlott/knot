package apiclient

import (
	"context"
	"time"
)

type TokenInfo struct {
	Id           string    `json:"token_id"`
	Name         string    `json:"name"`
	ExpiresAfter time.Time `json:"expires_after"`
	// Scopes restricts which endpoints the token can reach.
	// nil/empty = unrestricted. Non-empty = limited to the listed scopes.
	Scopes []string `json:"scopes,omitempty"`
}

type CreateTokenRequest struct {
	Name   string   `json:"name"`
	Scopes []string `json:"scopes,omitempty"`
}

type UpdateTokenRequest struct {
	Name   *string  `json:"name,omitempty"`
	Scopes *[]string `json:"scopes,omitempty"`
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

func (c *ApiClient) CreateToken(ctx context.Context, name string, scopes []string) (string, int, error) {
	request := &CreateTokenRequest{
		Name:   name,
		Scopes: scopes,
	}

	response := &CreateTokenResponse{}

	code, err := c.httpClient.Post(ctx, "/api/tokens", request, response, 201)
	if err != nil {
		return "", code, err
	}

	return response.TokenID, code, nil
}

func (c *ApiClient) UpdateToken(ctx context.Context, tokenId string, name *string, scopes *[]string) (int, error) {
	request := &UpdateTokenRequest{
		Name:   name,
		Scopes: scopes,
	}
	return c.httpClient.Put(ctx, "/api/tokens/"+tokenId, request, nil, 200)
}
