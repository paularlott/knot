package oauth2

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/util/rest"
)

// Dynamic client registration (RFC 7591) for MCP clients such as Claude's
// custom connectors, which register themselves before starting the
// authorization code flow.
//
// Registration is stateless: the client_id carries the client's name and
// redirect URIs, signed with a key derived from the zone's encryption key,
// so every server in the zone recognises it without storing anything.
// Authorize then only accepts a registered client's own redirect URIs.

const (
	registeredClientPrefix = "kc_"
	maxRedirectURIs        = 10
	maxRedirectURILength   = 2048
	maxClientNameLength    = 100
)

type registeredClient struct {
	Name         string   `json:"n"`
	RedirectURIs []string `json:"r"`
}

type clientRegistrationRequest struct {
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	Scope                   string   `json:"scope"`
}

type clientRegistrationResponse struct {
	ClientId                string   `json:"client_id"`
	ClientIdIssuedAt        int64    `json:"client_id_issued_at"`
	ClientName              string   `json:"client_name,omitempty"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	Scope                   string   `json:"scope,omitempty"`
}

// HandleRegister handles the dynamic client registration endpoint.
func HandleRegister(w http.ResponseWriter, r *http.Request) {
	var req clientRegistrationRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&req); err != nil {
		writeRegistrationError(w, r, "invalid_client_metadata", "Invalid registration request")
		return
	}

	if len(req.RedirectURIs) == 0 || len(req.RedirectURIs) > maxRedirectURIs {
		writeRegistrationError(w, r, "invalid_redirect_uri", "Between 1 and 10 redirect_uris are required")
		return
	}
	for _, uri := range req.RedirectURIs {
		if err := validateRedirectURI(uri); err != nil {
			writeRegistrationError(w, r, "invalid_redirect_uri", err.Error())
			return
		}
	}

	// Only public clients using the authorization code flow are supported.
	if req.TokenEndpointAuthMethod != "" && req.TokenEndpointAuthMethod != "none" {
		writeRegistrationError(w, r, "invalid_client_metadata", "Only token_endpoint_auth_method none is supported")
		return
	}
	for _, gt := range req.GrantTypes {
		if gt != "authorization_code" && gt != "refresh_token" {
			writeRegistrationError(w, r, "invalid_client_metadata", "Unsupported grant type: "+gt)
			return
		}
	}
	for _, rt := range req.ResponseTypes {
		if rt != "code" {
			writeRegistrationError(w, r, "invalid_client_metadata", "Unsupported response type: "+rt)
			return
		}
	}

	name := strings.TrimSpace(req.ClientName)
	if len(name) > maxClientNameLength {
		name = name[:maxClientNameLength]
	}

	clientId, err := encodeClientId(registeredClient{Name: name, RedirectURIs: req.RedirectURIs})
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: "server_error"})
		return
	}

	rest.WriteResponse(http.StatusCreated, w, r, clientRegistrationResponse{
		ClientId:                clientId,
		ClientIdIssuedAt:        time.Now().Unix(),
		ClientName:              name,
		RedirectURIs:            req.RedirectURIs,
		GrantTypes:              []string{"authorization_code", "refresh_token"},
		ResponseTypes:           []string{"code"},
		TokenEndpointAuthMethod: "none",
		Scope:                   strings.Join(grantedScopes(req.Scope), " "),
	})
}

func writeRegistrationError(w http.ResponseWriter, r *http.Request, code, description string) {
	rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{
		Error:            code,
		ErrorDescription: description,
	})
}

// validateRedirectURI accepts https URIs, and http only to loopback
// addresses for native clients (RFC 8252).
func validateRedirectURI(uri string) error {
	if len(uri) > maxRedirectURILength {
		return errors.New("redirect_uri is too long")
	}
	u, err := url.Parse(uri)
	if err != nil || u.Host == "" || u.Fragment != "" {
		return errors.New("invalid redirect_uri: " + uri)
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		host := u.Hostname()
		if host == "localhost" {
			return nil
		}
		if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
			return nil
		}
		return errors.New("http redirect_uri must use a loopback address: " + uri)
	default:
		return errors.New("redirect_uri must use https: " + uri)
	}
}

// clientSigningKey derives the key that signs registered client IDs from the
// zone's encryption key, so it is the same on every server in the zone.
func clientSigningKey() []byte {
	mac := hmac.New(sha256.New, []byte(config.GetServerConfig().EncryptionKey))
	mac.Write([]byte("knot-oauth2-client-registration"))
	return mac.Sum(nil)
}

func signClientPayload(payload string) string {
	mac := hmac.New(sha256.New, clientSigningKey())
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func encodeClientId(client registeredClient) (string, error) {
	data, err := json.Marshal(client)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(data)
	return registeredClientPrefix + payload + "." + signClientPayload(payload), nil
}

func decodeClientId(clientId string) (*registeredClient, error) {
	payload, sig, ok := strings.Cut(strings.TrimPrefix(clientId, registeredClientPrefix), ".")
	if !ok || !hmac.Equal([]byte(sig), []byte(signClientPayload(payload))) {
		return nil, errors.New("unknown client_id")
	}
	data, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return nil, errors.New("unknown client_id")
	}
	var client registeredClient
	if err := json.Unmarshal(data, &client); err != nil {
		return nil, errors.New("unknown client_id")
	}
	return &client, nil
}

// ValidateClient checks a client_id and redirect_uri pair. A registered
// client must use one of the redirect URIs it registered; other client IDs
// are accepted as unregistered public clients, as before registration was
// supported. It returns the client's registered name, if any.
func ValidateClient(clientId, redirectURI string) (string, error) {
	if !strings.HasPrefix(clientId, registeredClientPrefix) {
		return "", nil
	}
	client, err := decodeClientId(clientId)
	if err != nil {
		return "", err
	}
	for _, uri := range client.RedirectURIs {
		if uri == redirectURI {
			return client.Name, nil
		}
	}
	return "", errors.New("redirect_uri is not registered for this client")
}
