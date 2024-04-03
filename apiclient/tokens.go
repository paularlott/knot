package apiclient

import "time"

type TokenInfo struct {
	Id           string    `json:"token_id"`
	Name         string    `json:"name"`
	ExpiresAfter time.Time `json:"expires_at"`
}

type CreateTokenRequest struct {
	Name string `json:"name"`
}

type CreateTokenResponse struct {
	Status  bool   `json:"status"`
	TokenID string `json:"token_id"`
}

func (c *ApiClient) GetTokens() (*[]TokenInfo, int, error) {
	response := &[]TokenInfo{}

	code, err := c.httpClient.Get("/api/v1/tokens", response)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}

func (c *ApiClient) DeleteToken(tokenId string) (int, error) {
	return c.httpClient.Delete("/api/v1/tokens/"+tokenId, nil, nil, 200)
}

func (c *ApiClient) CreateToken(name string) (string, int, error) {
	request := &CreateTokenRequest{
		Name: name,
	}

	response := &CreateTokenResponse{}

	code, err := c.httpClient.Post("/api/v1/tokens", request, response, 201)
	if err != nil {
		return "", code, err
	}

	return response.TokenID, code, nil
}
