package apiclient

type AuthLoginRequest struct {
	Password string `json:"password"`
	Email    string `json:"email"`
}

type AuthLoginResponse struct {
	Status bool   `json:"status"`
	Token  string `json:"token"`
}

type AuthLogoutResponse struct {
	Status bool `json:"status"`
}

func (c *ApiClient) Login(email string, password string) (string, int, error) {
	request := AuthLoginRequest{
		Email:    email,
		Password: password,
	}
	response := AuthLoginResponse{}

	code, err := c.httpClient.Post("/api/v1/auth", &request, &response, 200)
	if err != nil {
		return "", code, err
	}

	return response.Token, code, nil
}

func (c *ApiClient) Logout() error {
	response := AuthLogoutResponse{}

	_, err := c.httpClient.Post("/api/v1/auth/logout", nil, &response, 200)
	if err != nil {
		return err
	}

	return nil
}
