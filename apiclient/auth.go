package apiclient

type AuthLoginRequest struct {
	Password string `json:"password"`
	Email    string `json:"email"`
	TOTPCode string `json:"totp_code"`
}

type AuthLoginResponse struct {
	Status     bool   `json:"status"`
	Token      string `json:"token"`
	TOTPSecret string `json:"totp_secret"`
}

type AuthLogoutResponse struct {
	Status bool `json:"status"`
}

type UsingTOTPResponse struct {
	UsingTOTP bool `json:"using_totp"`
}

func (c *ApiClient) Login(email string, password string, totpCode string) (string, string, int, error) {
	request := AuthLoginRequest{
		Email:    email,
		Password: password,
		TOTPCode: totpCode,
	}
	response := AuthLoginResponse{}

	code, err := c.httpClient.Post("/api/auth", &request, &response, 200)
	if err != nil {
		return "", "", code, err
	}

	return response.Token, response.TOTPSecret, code, nil
}

func (c *ApiClient) Logout() error {
	response := AuthLogoutResponse{}

	_, err := c.httpClient.Post("/api/auth/logout", nil, &response, 200)
	if err != nil {
		return err
	}

	return nil
}

// Login to the server using a user ID and token
func (c *ApiClient) LoginUserToken(userId string, token string) error {
	response := AuthLoginResponse{}

	_, err := c.httpClient.Post("/api/auth/user", nil, &response, 200)
	if err != nil {
		return err
	}

	return nil
}

func (c *ApiClient) UsingTOTP() (bool, int, error) {
	response := UsingTOTPResponse{}

	code, err := c.httpClient.Get("/api/auth/using-totp", &response)
	if err != nil {
		return false, code, err
	}

	return response.UsingTOTP, code, nil
}
