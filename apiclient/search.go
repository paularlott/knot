package apiclient

import (
	"context"
	"net/url"
)

// SearchHit is a single matched entity in a global search response. The client
// maps the enclosing result group (spaces, templates, …) to a destination page.
type SearchHit struct {
	Id          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// SearchResults is the grouped, permission/zone/ownership-scoped response from
// GET /api/search. Each slice holds at most a capped number of matches; empty
// groups are omitted from the JSON.
type SearchResults struct {
	Query      string      `json:"query"`
	Spaces     []SearchHit `json:"spaces,omitempty"`
	Templates  []SearchHit `json:"templates,omitempty"`
	Variables  []SearchHit `json:"variables,omitempty"`
	Volumes    []SearchHit `json:"volumes,omitempty"`
	Stacks     []SearchHit `json:"stacks,omitempty"`
	Scripts    []SearchHit `json:"scripts,omitempty"`
	Skills     []SearchHit `json:"skills,omitempty"`
	Commands   []SearchHit `json:"commands,omitempty"`
	MCPServers []SearchHit `json:"mcp_servers,omitempty"`
	EventSinks []SearchHit `json:"event_sinks,omitempty"`
	Users      []SearchHit `json:"users,omitempty"`
	Groups     []SearchHit `json:"groups,omitempty"`
	Roles      []SearchHit `json:"roles,omitempty"`
	Tokens     []SearchHit `json:"tokens,omitempty"`
}

// Search runs a global keyword search across the entity types the caller may
// see. The server validates permissions, ownership and zone.
func (c *ApiClient) Search(ctx context.Context, q string) (*SearchResults, error) {
	response := SearchResults{}
	_, err := c.httpClient.Get(ctx, "/api/search?q="+url.QueryEscape(q), &response)
	if err != nil {
		return nil, err
	}
	return &response, nil
}
