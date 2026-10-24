package oauth2

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/devicegrant"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/knot/internal/util/validate"

	"github.com/paularlott/knot/internal/log"
)

// The device authorization grant (RFC 8628) signs in `knot connect`: the CLI
// posts to /device_authorization and shows the user code, the user approves
// it at /device in the web interface (signing in there however the server
// allows), and the CLI polls /token with the device code until it gets an
// API token.
//
// Only the knot CLI may use the grant. The token it gets is an ordinary,
// unrestricted API token named by the CLI, the same as one created in the
// web interface.

const (
	// CLIClientId is the client_id of the knot CLI.
	CLIClientId = "knot-cli"

	GrantTypeDeviceCode = "urn:ietf:params:oauth:grant-type:device_code"
)

type DeviceAuthorizationResponse struct {
	DeviceCode              string `json:"device_code" msgpack:"device_code"`
	UserCode                string `json:"user_code" msgpack:"user_code"`
	VerificationURI         string `json:"verification_uri" msgpack:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete" msgpack:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in" msgpack:"expires_in"`
	Interval                int    `json:"interval" msgpack:"interval"`
}

// HandleDeviceAuthorization handles the device authorization endpoint
// (RFC 8628 section 3.1). Besides client_id it takes token_name, the name
// the API token is given once approved.
func HandleDeviceAuthorization(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	if err := r.ParseForm(); err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{
			Error:            "invalid_request",
			ErrorDescription: "Failed to parse form data",
		})
		return
	}

	if r.PostFormValue("client_id") != CLIClientId {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{
			Error:            "unauthorized_client",
			ErrorDescription: "Only the knot CLI may use the device authorization grant",
		})
		return
	}

	tokenName := r.PostFormValue("token_name")
	if !validate.TokenName(tokenName) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{
			Error:            "invalid_request",
			ErrorDescription: "Invalid token_name",
		})
		return
	}

	grant, err := devicegrant.New(tokenName)
	if errors.Is(err, devicegrant.ErrTooMany) {
		rest.WriteResponse(http.StatusServiceUnavailable, w, r, ErrorResponse{
			Error:            "slow_down",
			ErrorDescription: "Too many device sign-ins are waiting; try again in a few minutes",
		})
		return
	}
	if err != nil {
		log.WithGroup("oauth2").WithError(err).Error("failed to start device sign-in")
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: "server_error"})
		return
	}
	service.GetTransport().GossipDeviceGrant(grant)

	userCode := devicegrant.FormatUserCode(grant.UserCode)
	verificationURI := BaseURL(r) + "/device"
	w.Header().Set("Cache-Control", "no-store")
	rest.WriteResponse(http.StatusOK, w, r, DeviceAuthorizationResponse{
		DeviceCode:              grant.DeviceCode,
		UserCode:                userCode,
		VerificationURI:         verificationURI,
		VerificationURIComplete: verificationURI + "?user_code=" + url.QueryEscape(userCode),
		ExpiresIn:               int(devicegrant.TTL.Seconds()),
		Interval:                devicegrant.Interval,
	})
}

// handleDeviceCodeGrant is the token endpoint's device_code grant (RFC 8628
// section 3.4): authorization_pending until the user decides, then the token
// once, or access_denied.
func handleDeviceCodeGrant(w http.ResponseWriter, r *http.Request) {
	if r.FormValue("client_id") != CLIClientId {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "invalid_client"})
		return
	}

	deviceCode := r.FormValue("device_code")
	if deviceCode == "" {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{
			Error:            "invalid_request",
			ErrorDescription: "Missing device_code parameter",
		})
		return
	}

	grant, slowDown := devicegrant.ByDeviceCode(deviceCode)
	if grant == nil || grant.Status == devicegrant.StatusUsed {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{
			Error:            "expired_token",
			ErrorDescription: "The device code has expired; start again",
		})
		return
	}

	switch grant.Status {
	case devicegrant.StatusPending:
		errCode := "authorization_pending"
		if slowDown {
			errCode = "slow_down"
		}
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: errCode})
		return

	case devicegrant.StatusDenied:
		markUsed(grant)
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{
			Error:            "access_denied",
			ErrorDescription: "The sign-in was denied",
		})
		return
	}

	// Approved: the token was made when the user approved, and is handed
	// out once.
	markUsed(grant)
	w.Header().Set("Cache-Control", "no-store")
	rest.WriteResponse(http.StatusOK, w, r, TokenResponse{
		AccessToken: grant.TokenId,
		TokenType:   "Bearer",
		ExpiresIn:   int(model.MaxTokenAge.Seconds()),
	})
}

func markUsed(grant *devicegrant.Grant) {
	grant.Status = devicegrant.StatusUsed
	devicegrant.Apply(grant)
	service.GetTransport().GossipDeviceGrant(grant)
}
