package oauth2

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/util/rest"

	"github.com/paularlott/knot/internal/log"
)

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

type ErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description,omitempty"`
}

// HandleAuthorize handles the OAuth2 authorization endpoint
func HandleAuthorize(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	responseType := r.URL.Query().Get("response_type")
	clientId := r.URL.Query().Get("client_id")
	redirectURI := r.URL.Query().Get("redirect_uri")
	// scope := r.URL.Query().Get("scope") // Future use
	// state := r.URL.Query().Get("state") // Future use

	// Validate required parameters
	if responseType != "code" {
		http.Error(w, "unsupported_response_type", http.StatusBadRequest)
		return
	}

	if clientId == "" {
		http.Error(w, "invalid_request: missing client_id", http.StatusBadRequest)
		return
	}

	if redirectURI == "" {
		http.Error(w, "invalid_request: missing redirect_uri", http.StatusBadRequest)
		return
	}

	// Validate redirect URI format
	if _, err := url.Parse(redirectURI); err != nil {
		http.Error(w, "invalid_request: invalid redirect_uri", http.StatusBadRequest)
		return
	}

	// PKCE (RFC 7636) with S256 is required for all clients. MCP clients
	// implement PKCE per the MCP specification, so only non-conformant
	// clients are excluded.
	if reason := validatePKCEChallenge(r.URL.Query().Get("code_challenge"), r.URL.Query().Get("code_challenge_method")); reason != "" {
		http.Error(w, "invalid_request: "+reason, http.StatusBadRequest)
		return
	}

	// Check if user is authenticated
	user := r.Context().Value("user")
	if user == nil {
		// Redirect to login with OAuth params preserved
		loginURL := "/login?redirect=" + url.QueryEscape(r.URL.String())
		http.Redirect(w, r, loginURL, http.StatusSeeOther)
		return
	}

	// Show the grant page
	http.Redirect(w, r, "/oauth/grant?"+r.URL.RawQuery, http.StatusSeeOther)
}

// HandleGrant handles the OAuth2 grant approval
func HandleGrant(w http.ResponseWriter, r *http.Request) {
	logger := log.WithGroup("oauth2")

	// Handle grant approval/denial
	action := r.FormValue("action")
	clientId := r.FormValue("client_id")
	redirectURI := r.FormValue("redirect_uri")
	scope := r.FormValue("scope")
	state := r.FormValue("state")
	codeChallenge := r.FormValue("code_challenge")
	codeChallengeMethod := r.FormValue("code_challenge_method")

	user := r.Context().Value("user").(*model.User)

	if action == "approve" {
		// Never issue a code that isn't PKCE-bound, even if the approve
		// request is forged directly rather than coming from the grant page.
		if reason := validatePKCEChallenge(codeChallenge, codeChallengeMethod); reason != "" {
			http.Error(w, "invalid_request: "+reason, http.StatusBadRequest)
			return
		}

		// Create authorization code
		authCodeStore := GetAuthCodeStore()
		authCode, err := authCodeStore.CreateAuthCode(user.Id, clientId, redirectURI, scope, codeChallenge, codeChallengeMethod)
		if err != nil {
			logger.WithError(err).Error("failed to create auth code")
			http.Error(w, "server_error", http.StatusInternalServerError)
			return
		}

		// Redirect back to client with code
		redirectURL, _ := url.Parse(redirectURI)
		q := redirectURL.Query()
		q.Set("code", authCode.Code)
		if state != "" {
			q.Set("state", state)
		}
		redirectURL.RawQuery = q.Encode()

		http.Redirect(w, r, redirectURL.String(), http.StatusSeeOther)
	} else {
		// User denied access
		redirectURL, _ := url.Parse(redirectURI)
		q := redirectURL.Query()
		q.Set("error", "access_denied")
		if state != "" {
			q.Set("state", state)
		}
		redirectURL.RawQuery = q.Encode()

		http.Redirect(w, r, redirectURL.String(), http.StatusSeeOther)
	}
}

// HandleToken handles the OAuth2 token endpoint
func HandleToken(w http.ResponseWriter, r *http.Request) {
	logger := log.WithGroup("oauth2")

	// Debug: Log request details
	logger.Debug("token request headers",
		"content_type", r.Header.Get("Content-Type"),
		"content_length", r.Header.Get("Content-Length"))

	// Parse form data first
	err := r.ParseForm()
	if err != nil {
		logger.WithError(err).Error("failed to parse form")
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{
			Error:            "invalid_request",
			ErrorDescription: "Failed to parse form data",
		})
		return
	}

	grantType := r.FormValue("grant_type")
	code := r.FormValue("code")
	redirectURI := r.FormValue("redirect_uri")

	// Debug logging
	logger.Debug("token request parameters",
		"grant_type", grantType,
		"code", code,
		"redirect_uri", redirectURI,
		"form", r.Form)

	// Validate grant type
	switch grantType {
	case "authorization_code":
		handleAuthorizationCodeGrant(w, r)
	case "refresh_token":
		handleRefreshTokenGrant(w, r)
	default:
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{
			Error: "unsupported_grant_type",
		})
	}
}

func handleAuthorizationCodeGrant(w http.ResponseWriter, r *http.Request) {
	logger := log.WithGroup("oauth2")

	code := r.FormValue("code")
	redirectURI := r.FormValue("redirect_uri")

	// Validate required parameters
	if code == "" || redirectURI == "" {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{
			Error:            "invalid_request",
			ErrorDescription: "Missing required parameters",
		})
		return
	}

	// Exchange code for token
	authCodeStore := GetAuthCodeStore()
	authCode, valid := authCodeStore.ConsumeAuthCode(code)
	if !valid {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{
			Error:            "invalid_grant",
			ErrorDescription: "Invalid or expired authorization code",
		})
		return
	}

	// Validate redirect URI matches
	if authCode.RedirectURI != redirectURI {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{
			Error:            "invalid_grant",
			ErrorDescription: "Redirect URI mismatch",
		})
		return
	}

	// PKCE (RFC 7636): the verifier must match the challenge bound to the
	// code at authorize time. The code has already been consumed, so a wrong
	// verifier also burns the code.
	if !verifyPKCE(authCode.CodeChallenge, authCode.CodeChallengeMethod, r.FormValue("code_verifier")) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{
			Error:            "invalid_grant",
			ErrorDescription: "PKCE verification failed",
		})
		return
	}

	name := authCode.ClientId
	if u, err := url.Parse(authCode.RedirectURI); err == nil {
		name = u.Hostname()
	}

	// Create access token. OAuth-issued tokens are always scoped — by
	// default to the MCP endpoint — so a code phished through a crafted
	// redirect_uri can never yield an unrestricted account token.
	tokenName := "OAuth2 Token for " + name
	token := model.NewToken(tokenName, authCode.UserId)
	token.Scopes = grantedScopes(authCode.Scope)
	token.RefreshToken = true

	db := database.GetInstance()
	err := db.SaveToken(token)
	if err != nil {
		logger.WithError(err).Error("failed to save token")
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{
			Error: "server_error",
		})
		return
	}

	service.GetTransport().GossipToken(token)

	// Return token response
	expiresIn := int(model.MaxTokenAge.Seconds())
	rest.WriteResponse(http.StatusOK, w, r, TokenResponse{
		AccessToken:  token.Id,
		TokenType:    "Bearer",
		ExpiresIn:    expiresIn,
		RefreshToken: token.Id,
		Scope:        strings.Join(token.Scopes, " "),
	})
}

func handleRefreshTokenGrant(w http.ResponseWriter, r *http.Request) {
	refreshTokenId := r.FormValue("refresh_token")

	if refreshTokenId == "" {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{
			Error:            "invalid_request",
			ErrorDescription: "Missing refresh_token parameter",
		})
		return
	}

	db := database.GetInstance()

	// Get the refresh token
	refreshToken, err := db.GetToken(refreshTokenId)
	if err != nil || refreshToken.IsDeleted {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{
			Error:            "invalid_grant",
			ErrorDescription: "Invalid refresh token",
		})
		return
	}

	// Only tokens issued via the OAuth2 flow may be refreshed, and only
	// while still unexpired — an expired or UI-created token is never
	// extended (otherwise expiry would be unenforceable).
	if !refreshToken.RefreshToken || !refreshToken.ExpiresAfter.After(time.Now().UTC()) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{
			Error:            "invalid_grant",
			ErrorDescription: "Invalid refresh token",
		})
		return
	}

	// Save the token to extend its life
	expiresAfter := time.Now().Add(model.MaxTokenAge)
	refreshToken.ExpiresAfter = expiresAfter.UTC()
	refreshToken.UpdatedAt = hlc.Now()
	db.SaveToken(refreshToken)
	service.GetTransport().GossipToken(refreshToken)

	// Return new token response
	expiresIn := int(model.MaxTokenAge.Seconds())
	rest.WriteResponse(http.StatusOK, w, r, TokenResponse{
		AccessToken:  refreshToken.Id,
		TokenType:    "Bearer",
		ExpiresIn:    expiresIn,
		RefreshToken: refreshToken.Id,
		Scope:        strings.Join(refreshToken.Scopes, " "),
	})
}

type AuthorizationServerMetadata struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	ResponseTypesSupported            []string `json:"response_types_supported"`
	GrantTypesSupported               []string `json:"grant_types_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
	ScopesSupported                   []string `json:"scopes_supported,omitempty"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported,omitempty"`
}

func HandleAuthorizationServerMetadata(w http.ResponseWriter, r *http.Request) {
	cfg := config.GetServerConfig()

	// Build the base URL from the server configuration
	baseURL := cfg.URL
	if baseURL == "" {
		// Fallback to constructing from request
		scheme := "https"
		if r.TLS == nil {
			scheme = "http"
		}
		baseURL = scheme + "://" + r.Host
	}

	metadata := AuthorizationServerMetadata{
		Issuer:                baseURL,
		AuthorizationEndpoint: baseURL + "/authorize",
		TokenEndpoint:         baseURL + "/token",
		ResponseTypesSupported: []string{
			"code",
		},
		GrantTypesSupported: []string{
			"authorization_code",
			"refresh_token",
		},
		TokenEndpointAuthMethodsSupported: []string{
			"none", // For public clients
		},
		ScopesSupported: []string{
			model.ScopeMCP,
		},
		CodeChallengeMethodsSupported: []string{
			"S256",
		},
	}

	w.Header().Set("Content-Type", "application/json")
	rest.WriteResponse(http.StatusOK, w, r, metadata)
}

type OpenIDConfiguration struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	UserinfoEndpoint                  string   `json:"userinfo_endpoint"`
	JwksURI                           string   `json:"jwks_uri"`
	ResponseTypesSupported            []string `json:"response_types_supported"`
	SubjectTypesSupported             []string `json:"subject_types_supported"`
	IDTokenSigningAlgValuesSupported  []string `json:"id_token_signing_alg_values_supported"`
	ScopesSupported                   []string `json:"scopes_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
	ClaimsSupported                   []string `json:"claims_supported"`
}

// validatePKCEChallenge checks the code_challenge and code_challenge_method
// parameters. It returns a non-empty error reason when they don't conform to
// PKCE S256 (RFC 7636), and an empty string when valid.
func validatePKCEChallenge(challenge, method string) string {
	if challenge == "" {
		return "missing code_challenge"
	}
	if method == "" {
		// RFC 7636 defaults the method to "plain", which is not accepted.
		return "missing code_challenge_method (S256 required)"
	}
	if method != "S256" {
		return "unsupported code_challenge_method (S256 required)"
	}
	// A SHA-256 challenge is 43 unpadded base64url characters; allow some
	// slack for unusual client encodings but reject clearly malformed values.
	if len(challenge) < 43 || len(challenge) > 128 {
		return "invalid code_challenge"
	}
	return ""
}

// verifyPKCE checks a code_verifier against the stored challenge using the
// stored method (RFC 7636 section 4.6). Only S256 is supported; the plain
// method is never honored.
func verifyPKCE(challenge, method, verifier string) bool {
	if challenge == "" || verifier == "" || method != "S256" {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	computed := base64.RawURLEncoding.EncodeToString(sum[:])
	return hmac.Equal([]byte(computed), []byte(challenge))
}

// grantedScopes maps the requested OAuth scope string to knot token scopes.
// Only scopes knot knows are granted; unknown, missing, or empty requests
// default to the mcp scope. OAuth-issued tokens are never unrestricted.
func grantedScopes(scope string) []string {
	var scopes []string
	for _, s := range strings.Fields(scope) {
		if model.IsKnownTokenScope(s) {
			scopes = append(scopes, s)
		}
	}
	if len(scopes) == 0 {
		scopes = []string{model.ScopeMCP}
	}
	return scopes
}
