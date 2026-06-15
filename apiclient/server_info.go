package apiclient

import "context"

// ServerInfoResponse exposes server-wide configuration that clients (such as
// the VS Code extension) need but that is not user-specific.
type ServerInfoResponse struct {
	// Version is the knot server version string.
	Version string `json:"version"`
	// WildcardDomain is the server's wildcard domain used to build space
	// web-port dev URLs (e.g. "*.knot.example.com"). Empty when not configured.
	WildcardDomain string `json:"wildcard_domain"`
}

// GetServerInfo returns server-wide information.
func (c *ApiClient) GetServerInfo(ctx context.Context) (*ServerInfoResponse, int, error) {
	response := &ServerInfoResponse{}
	code, err := c.httpClient.Get(ctx, "/api/server-info", response)
	if err != nil {
		return nil, code, err
	}
	return response, code, nil
}
