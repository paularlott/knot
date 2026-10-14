package apiclient

import (
	"context"
	"net/url"
)

// SpaceRestrictBucket is one bucket a restricted space may still reach.
// Name is as the space's owner sees it: the short name for their own
// buckets, owner--name for buckets shared with them.
type SpaceRestrictBucket struct {
	Name     string `json:"name" msgpack:"name"`
	ReadOnly bool   `json:"read_only" msgpack:"read_only"`
}

// SpaceRestrictRequest adds restrictions to a space. Restrictions only ever
// tighten: anything already restricted stays restricted. Buckets, when
// given, is an allowlist: every other bucket is dropped.
type SpaceRestrictRequest struct {
	NoSecrets bool                  `json:"no_secrets" msgpack:"no_secrets"`
	Files     string                `json:"files" msgpack:"files"` // "", "ro" or "none"
	Buckets   []SpaceRestrictBucket `json:"buckets,omitempty" msgpack:"buckets,omitempty"`
	// Lock locks the space's processes out of knot, except restarting or
	// shutting the space down.
	Lock bool `json:"lock" msgpack:"lock"`
}

// SpaceRestrictions reports what a space's own credential is restricted
// to, including what its template switches off.
type SpaceRestrictions struct {
	SpaceId    string `json:"space_id" msgpack:"space_id"`
	SpaceName  string `json:"space_name" msgpack:"space_name"`
	Restricted bool   `json:"restricted" msgpack:"restricted"`
	NoSecrets  bool   `json:"no_secrets" msgpack:"no_secrets"`
	Locked     bool   `json:"locked" msgpack:"locked"`
	Files      string `json:"files" msgpack:"files"` // "", "ro" or "none"
	// LimitBuckets is set when only the listed buckets may be reached.
	LimitBuckets      bool                  `json:"limit_buckets" msgpack:"limit_buckets"`
	Buckets           []SpaceRestrictBucket `json:"buckets" msgpack:"buckets"`
	TemplateNoFiles   bool                  `json:"template_no_files" msgpack:"template_no_files"`
	TemplateNoSecrets bool                  `json:"template_no_secrets" msgpack:"template_no_secrets"`
}

// RestrictSpace adds restrictions to a space and returns them all.
func (c *ApiClient) RestrictSpace(ctx context.Context, spaceId string, request *SpaceRestrictRequest) (*SpaceRestrictions, int, error) {
	response := &SpaceRestrictions{}
	code, err := c.httpClient.Post(ctx, "/api/spaces/"+url.PathEscape(spaceId)+"/restrict", request, response, 200)
	if err != nil {
		return nil, code, err
	}
	return response, code, nil
}

// GetSpaceRestrictions returns a space's restrictions.
func (c *ApiClient) GetSpaceRestrictions(ctx context.Context, spaceId string) (*SpaceRestrictions, int, error) {
	response := &SpaceRestrictions{}
	code, err := c.httpClient.Get(ctx, "/api/spaces/"+url.PathEscape(spaceId)+"/restrictions", response)
	if err != nil {
		return nil, code, err
	}
	return response, code, nil
}

// ClearSpaceRestrictions lifts all of a space's restrictions. It is refused
// from inside a space: there, only restarting the space lifts them.
func (c *ApiClient) ClearSpaceRestrictions(ctx context.Context, spaceId string) (*SpaceRestrictions, int, error) {
	response := &SpaceRestrictions{}
	code, err := c.httpClient.Delete(ctx, "/api/spaces/"+url.PathEscape(spaceId)+"/restrictions", nil, response, 200)
	if err != nil {
		return nil, code, err
	}
	return response, code, nil
}
