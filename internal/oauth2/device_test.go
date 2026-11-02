package oauth2

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/devicegrant"
	"github.com/paularlott/knot/internal/service"
)

type deviceGossipTransport struct {
	service.Transport
	grants int
}

func (t *deviceGossipTransport) GossipDeviceGrant(*devicegrant.Grant) { t.grants++ }

func postForm(handler http.HandlerFunc, path string, form url.Values) (*httptest.ResponseRecorder, map[string]any) {
	req := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	handler(rec, req)
	body := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return rec, body
}

func startDevice(t *testing.T) *DeviceAuthorizationResponse {
	t.Helper()
	prevCfg := config.GetServerConfig()
	config.SetServerConfig(&config.ServerConfig{URL: "https://knot.example.com"})
	t.Cleanup(func() { config.SetServerConfig(prevCfg) })

	rec, _ := postForm(HandleDeviceAuthorization, "/device_authorization", url.Values{
		"client_id":  {CLIClientId},
		"token_name": {"knot client test"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("device authorization = %d (%s), want 200", rec.Code, rec.Body.String())
	}
	var auth DeviceAuthorizationResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &auth); err != nil {
		t.Fatal(err)
	}
	return &auth
}

func pollDevice(deviceCode string) (*httptest.ResponseRecorder, map[string]any) {
	return postForm(HandleToken, "/token", url.Values{
		"grant_type":  {GrantTypeDeviceCode},
		"client_id":   {CLIClientId},
		"device_code": {deviceCode},
	})
}

func TestDeviceAuthorizationRequiresCLIClient(t *testing.T) {
	service.SetTransport(&deviceGossipTransport{})
	t.Cleanup(func() { service.SetTransport(nil) })

	rec, body := postForm(HandleDeviceAuthorization, "/device_authorization", url.Values{
		"client_id":  {"someone-else"},
		"token_name": {"knot client test"},
	})
	if rec.Code != http.StatusBadRequest || body["error"] != "unauthorized_client" {
		t.Errorf("other client = %d %v, want 400 unauthorized_client", rec.Code, body)
	}

	rec, body = postForm(HandleDeviceAuthorization, "/device_authorization", url.Values{
		"client_id": {CLIClientId},
	})
	if rec.Code != http.StatusBadRequest || body["error"] != "invalid_request" {
		t.Errorf("missing token_name = %d %v, want 400 invalid_request", rec.Code, body)
	}
}

func TestDeviceCodeGrantApproved(t *testing.T) {
	transport := &deviceGossipTransport{}
	service.SetTransport(transport)
	t.Cleanup(func() { service.SetTransport(nil) })

	auth := startDevice(t)
	if auth.VerificationURI != "https://knot.example.com/device" || !strings.Contains(auth.VerificationURIComplete, "user_code=") {
		t.Errorf("verification URIs = %q, %q", auth.VerificationURI, auth.VerificationURIComplete)
	}

	if _, body := pollDevice(auth.DeviceCode); body["error"] != "authorization_pending" {
		t.Errorf("first poll = %v, want authorization_pending", body)
	}
	if _, body := pollDevice(auth.DeviceCode); body["error"] != "slow_down" {
		t.Errorf("immediate second poll = %v, want slow_down", body)
	}

	// The user approves (as web.HandleDeviceDecision does).
	grant, _ := devicegrant.ByUserCode("user-1", auth.UserCode)
	if grant == nil {
		t.Fatal("the user code should find the grant")
	}
	grant.Status = devicegrant.StatusApproved
	grant.UserId = "user-1"
	grant.TokenId = "tk_approved"
	devicegrant.Apply(grant)

	rec, body := pollDevice(auth.DeviceCode)
	if rec.Code != http.StatusOK || body["access_token"] != "tk_approved" {
		t.Fatalf("poll after approval = %d %v, want the token", rec.Code, body)
	}

	// The token is handed out once.
	if _, body := pollDevice(auth.DeviceCode); body["error"] != "expired_token" {
		t.Errorf("poll after collecting = %v, want expired_token", body)
	}
	if transport.grants == 0 {
		t.Error("device grant changes should be gossiped")
	}
}

func TestDeviceCodeGrantDenied(t *testing.T) {
	service.SetTransport(&deviceGossipTransport{})
	t.Cleanup(func() { service.SetTransport(nil) })

	auth := startDevice(t)
	grant, _ := devicegrant.ByUserCode("user-1", auth.UserCode)
	grant.Status = devicegrant.StatusDenied
	devicegrant.Apply(grant)

	if _, body := pollDevice(auth.DeviceCode); body["error"] != "access_denied" {
		t.Errorf("poll after denial = %v, want access_denied", body)
	}
	if _, body := pollDevice("no-such-code"); body["error"] != "expired_token" {
		t.Errorf("unknown device code = %v, want expired_token", body)
	}
}
