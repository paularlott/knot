package apiclient

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// The device authorization grant (RFC 8628) that `knot connect` signs in
// with. Its endpoints take form-encoded requests and answer in JSON, as the
// RFC requires, so they don't go through the REST client.

const deviceClientId = "knot-cli"

// DeviceAuthorization is the server's answer to starting a device sign-in.
type DeviceAuthorization struct {
	DeviceCode              string `json:"device_code" msgpack:"device_code"`
	UserCode                string `json:"user_code" msgpack:"user_code"`
	VerificationURI         string `json:"verification_uri" msgpack:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete" msgpack:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in" msgpack:"expires_in"`
	Interval                int    `json:"interval" msgpack:"interval"`
}

// ErrDeviceGrantUnsupported is returned when the server has no device
// authorization endpoint.
var ErrDeviceGrantUnsupported = errors.New("the server doesn't support device sign-in")

// DeviceClient talks to a server's device authorization and token endpoints.
type DeviceClient struct {
	baseURL string
	client  *http.Client
}

func NewDeviceClient(baseURL string, insecureSkipVerify bool) *DeviceClient {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: insecureSkipVerify}
	return &DeviceClient{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		client:  &http.Client{Transport: transport, Timeout: 30 * time.Second},
	}
}

type oauthError struct {
	Error            string `json:"error" msgpack:"error"`
	ErrorDescription string `json:"error_description" msgpack:"error_description"`
}

// Start begins a device sign-in for an API token with the given name.
func (c *DeviceClient) Start(ctx context.Context, tokenName string) (*DeviceAuthorization, error) {
	var auth DeviceAuthorization
	status, oe, err := c.post(ctx, "/device_authorization", url.Values{
		"client_id":  {deviceClientId},
		"token_name": {tokenName},
	}, &auth)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound || status == http.StatusMethodNotAllowed {
		return nil, ErrDeviceGrantUnsupported
	}
	if oe != nil {
		return nil, describeOAuthError(oe)
	}
	if auth.DeviceCode == "" || auth.UserCode == "" || auth.VerificationURI == "" {
		return nil, errors.New("the server's device sign-in answer is incomplete")
	}
	if auth.Interval <= 0 {
		auth.Interval = 5
	}
	return &auth, nil
}

// Wait polls the token endpoint until the user approves or denies the sign-in,
// it expires, or ctx ends, and returns the API token.
func (c *DeviceClient) Wait(ctx context.Context, auth *DeviceAuthorization) (string, error) {
	interval := time.Duration(auth.Interval) * time.Second
	deadline := time.Now().Add(time.Duration(auth.ExpiresIn) * time.Second)

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(interval):
		}
		if auth.ExpiresIn > 0 && time.Now().After(deadline) {
			return "", errors.New("the code expired before it was approved; run knot connect again")
		}

		var token struct {
			AccessToken string `json:"access_token" msgpack:"access_token"`
		}
		_, oe, err := c.post(ctx, "/token", url.Values{
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
			"client_id":   {deviceClientId},
			"device_code": {auth.DeviceCode},
		}, &token)
		if err != nil {
			return "", err
		}
		if oe == nil {
			if token.AccessToken == "" {
				return "", errors.New("the server returned no token")
			}
			return token.AccessToken, nil
		}

		switch oe.Error {
		case "authorization_pending":
		case "slow_down":
			interval += 5 * time.Second
		case "access_denied":
			return "", errors.New("the sign-in was denied")
		case "expired_token":
			return "", errors.New("the code expired before it was approved; run knot connect again")
		default:
			return "", describeOAuthError(oe)
		}
	}
}

// post sends a form and decodes a 200 answer into out. Any other answer
// with an OAuth error body comes back as oe.
func (c *DeviceClient) post(ctx context.Context, path string, form url.Values, out any) (int, *oauthError, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return resp.StatusCode, nil, err
	}

	if resp.StatusCode == http.StatusOK {
		if err := json.Unmarshal(body, out); err != nil {
			return resp.StatusCode, nil, fmt.Errorf("couldn't read the server's answer: %w", err)
		}
		return resp.StatusCode, nil, nil
	}

	var oe oauthError
	if json.Unmarshal(body, &oe) == nil && oe.Error != "" {
		return resp.StatusCode, &oe, nil
	}
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed {
		return resp.StatusCode, nil, nil
	}
	return resp.StatusCode, nil, fmt.Errorf("the server answered %s", resp.Status)
}

func describeOAuthError(oe *oauthError) error {
	if oe.ErrorDescription != "" {
		return fmt.Errorf("%s (%s)", oe.ErrorDescription, oe.Error)
	}
	return errors.New(oe.Error)
}
