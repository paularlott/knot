//go:build integration

package suites

import (
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/internal/util/rest"
)

// TestRepliesMatchAcrossFormats asks the same questions of the server as
// JSON and as msgpack (what the knot command uses) and requires the same
// answers. A field missing its msgpack tag on one side is lost in msgpack
// only, which nothing else would notice.
func TestRepliesMatchAcrossFormats(t *testing.T) {
	ctx, cancel := testCtx(120)
	defer cancel()
	spaceId := workspace(t)

	jsonClient := admin.Client // the harness pins JSON
	msgpackClient, err := apiclient.NewClient(server.BaseURL, admin.Token, true)
	if err != nil {
		t.Fatal(err)
	}
	user1Msgpack, err := apiclient.NewClient(server.BaseURL, user1.Token, true)
	if err != nil {
		t.Fatal(err)
	}

	// The server answers the knot command in msgpack and says its version;
	// a knot command from another minor version is turned away.
	get := func(userAgent string) (*http.Response, string) {
		req, _ := http.NewRequestWithContext(ctx, "GET", server.BaseURL+"/api/ping", nil)
		req.Header.Set("Accept", rest.ContentTypeMsgPack+", application/json")
		req.Header.Set("Authorization", "Bearer "+admin.Token)
		req.Header.Set("User-Agent", userAgent)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return resp, string(body)
	}
	resp, _ := get("knot v" + build.Version)
	if ct := resp.Header.Get("Content-Type"); resp.StatusCode != 200 || ct != rest.ContentTypeMsgPack || resp.Header.Get(build.VersionHeader) != build.Version {
		t.Fatalf("ping answered %d as %q, version %q", resp.StatusCode, ct, resp.Header.Get(build.VersionHeader))
	}
	resp, body := get("knot v0.1.0")
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "use a knot command from version "+build.MajorMinor(build.Version)) {
		t.Fatalf("a knot command from 0.1: %d %q, want it turned away", resp.StatusCode, body)
	}

	type call struct {
		name string
		as   func(c *apiclient.ApiClient) (any, error)
	}
	two := func(v any, _ int, err error) (any, error) { return v, err }
	calls := []call{
		{"ping", func(c *apiclient.ApiClient) (any, error) { return c.Ping(ctx) }},
		{"whoami", func(c *apiclient.ApiClient) (any, error) { return c.WhoAmI(ctx) }},
		{"users", func(c *apiclient.ApiClient) (any, error) { return c.GetUsers(ctx, "", "") }},
		{"user", func(c *apiclient.ApiClient) (any, error) { return c.GetUser(ctx, user1.Id) }},
		{"spaces", func(c *apiclient.ApiClient) (any, error) { return two(c.GetSpaces(ctx, user1.Id, false)) }},
		{"space", func(c *apiclient.ApiClient) (any, error) { return two(c.GetSpace(ctx, spaceId)) }},
		{"restrictions", func(c *apiclient.ApiClient) (any, error) { return two(c.GetSpaceRestrictions(ctx, spaceId)) }},
		{"templates", func(c *apiclient.ApiClient) (any, error) { return two(c.GetTemplates(ctx)) }},
		{"template", func(c *apiclient.ApiClient) (any, error) { return two(c.GetTemplate(ctx, templateId)) }},
		{"groups", func(c *apiclient.ApiClient) (any, error) { return two(c.GetGroups(ctx)) }},
		{"roles", func(c *apiclient.ApiClient) (any, error) { return two(c.GetRoles(ctx)) }},
		{"permissions", func(c *apiclient.ApiClient) (any, error) { return two(c.GetPermissions(ctx)) }},
		{"tokens", func(c *apiclient.ApiClient) (any, error) { return two(c.GetTokens(ctx)) }},
		{"template vars", func(c *apiclient.ApiClient) (any, error) { return two(c.GetTemplateVars(ctx)) }},
		{"volumes", func(c *apiclient.ApiClient) (any, error) { return two(c.GetVolumes(ctx)) }},
		{"pools", func(c *apiclient.ApiClient) (any, error) { return two(c.GetPools(ctx)) }},
		{"stack definitions", func(c *apiclient.ApiClient) (any, error) { return two(c.GetStackDefinitions(ctx)) }},
		{"icons", func(c *apiclient.ApiClient) (any, error) { return two(c.GetIcons(ctx)) }},
		{"server info", func(c *apiclient.ApiClient) (any, error) { return two(c.GetServerInfo(ctx)) }},
		{"cluster info", func(c *apiclient.ApiClient) (any, error) { return two(c.GetClusterInfo(ctx)) }},
		{"tunnels", func(c *apiclient.ApiClient) (any, error) { return two(c.GetTunnels(ctx)) }},
		{"commands", func(c *apiclient.ApiClient) (any, error) { return c.GetCommands(ctx) }},
		{"scripts", func(c *apiclient.ApiClient) (any, error) { return c.GetScripts(ctx) }},
		{"skills", func(c *apiclient.ApiClient) (any, error) { return c.GetSkills(ctx) }},
		{"methods", func(c *apiclient.ApiClient) (any, error) { return c.GetMethods(ctx) }},
		{"event sinks", func(c *apiclient.ApiClient) (any, error) { return c.GetEventSinks(ctx) }},
		{"nav preferences", func(c *apiclient.ApiClient) (any, error) { return c.GetOwnNavPreferences(ctx) }},
		{"file buckets", func(c *apiclient.ApiClient) (any, error) { return c.GetFileBuckets(ctx, true) }},
		{"audit logs", func(c *apiclient.ApiClient) (any, error) { return two(c.GetAuditLogs(ctx, 0, 20, nil)) }},
	}

	// user1's own space files, through a client that may read them.
	fileCalls := []call{
		{"find", func(c *apiclient.ApiClient) (any, error) {
			return c.Find(ctx, spaceId, apiclient.FindRequest{Path: ".", Recursive: true, Type: "any", IncludeMetadata: true, IncludeHash: true})
		}},
	}

	compare := func(name string, jsonC, msgpackC *apiclient.ApiClient, as func(c *apiclient.ApiClient) (any, error)) {
		t.Helper()
		before, err := as(jsonC)
		if err != nil {
			t.Logf("%s: skipped, JSON call failed: %v", name, err)
			return
		}
		got, err := as(msgpackC)
		if err != nil {
			t.Errorf("%s: msgpack call failed where JSON worked: %v", name, err)
			return
		}
		after, _ := as(jsonC)
		// Something may change between the calls (audit entries, last
		// seen); the msgpack answer must match the JSON one before or after.
		if !sameReply(before, got) && !sameReply(after, got) {
			b, _ := json.Marshal(before)
			g, _ := json.Marshal(got)
			t.Errorf("%s differs between formats\n json:    %s\n msgpack: %s", name, b, g)
		}
	}
	for _, c := range calls {
		compare(c.name, jsonClient, msgpackClient, c.as)
	}
	for _, c := range fileCalls {
		compare(c.name, user1.Client, user1Msgpack, c.as)
	}
}

// sameReply compares two decoded replies as JSON trees, with times compared
// as instants (JSON keeps the server's zone, msgpack carries only the time)
// and lists in any order (some, such as roles, come from a map).
func sameReply(a, b any) bool {
	return reflect.DeepEqual(normalisedTree(a), normalisedTree(b))
}

func normalisedTree(v any) any {
	data, err := json.Marshal(v)
	if err != nil {
		return err.Error()
	}
	var tree any
	json.Unmarshal(data, &tree)
	return normaliseTimes(tree)
}

func normaliseTimes(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			x[k] = normaliseTimes(child)
		}
	case []any:
		for i, child := range x {
			x[i] = normaliseTimes(child)
		}
		sort.Slice(x, func(i, j int) bool {
			a, _ := json.Marshal(x[i])
			b, _ := json.Marshal(x[j])
			return string(a) < string(b)
		})
	case string:
		if t, err := time.Parse(time.RFC3339Nano, x); err == nil {
			return t.UTC().Format(time.RFC3339Nano)
		}
	}
	return v
}
