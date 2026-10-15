package apiclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/api"
	"github.com/paularlott/knot/internal/util/rest"
)

// TestSpaceFileRequestsReachServer sends each space file and command
// request through a client and decodes it with the type the server's
// handler decodes it into, in both encodings. A field the client tags
// differently from the server arrives empty in msgpack — which is how
// knot space read-file once sent no path at all.
func TestSpaceFileRequestsReachServer(t *testing.T) {
	type call struct {
		name   string
		path   string // endpoint suffix
		into   func() any
		send   func(c *apiclient.ApiClient) // errors are expected: the reply is minimal
		expect any
	}
	mtimeMin := 1.5
	calls := []call{
		{"read range", "/files/read", func() any { return &api.ReadFileRequest{} },
			func(c *apiclient.ApiClient) { c.ReadSpaceFileRange(context.Background(), "sp", "/a.txt", 2, 3) },
			&api.ReadFileRequest{Path: "/a.txt", Offset: 2, Limit: 3}},
		{"read bytes", "/files/read", func() any { return &api.ReadFileRequest{} },
			func(c *apiclient.ApiClient) { c.ReadSpaceFileBytes(context.Background(), "sp", "/b.bin", 8, 16) },
			&api.ReadFileRequest{Path: "/b.bin", ByteOffset: 8, ByteLength: 16}},
		{"write", "/files/write", func() any { return &api.WriteFileRequest{} },
			func(c *apiclient.ApiClient) {
				c.WriteSpaceFileOpts(context.Background(), "sp", "/w.txt", "hi", "append", 123, 0640)
			},
			&api.WriteFileRequest{Path: "/w.txt", Content: "hi", Mode: "append", MtimeNs: 123, FilePerm: 0640}},
		{"symlink", "/files/write", func() any { return &api.WriteFileRequest{} },
			func(c *apiclient.ApiClient) { c.CreateSymlinkSpaceFile(context.Background(), "sp", "/l", "target") },
			&api.WriteFileRequest{Path: "/l", SymlinkTarget: "target"}},
		{"run", "/run-command", func() any { return &api.RunCommandRequest{} },
			func(c *apiclient.ApiClient) {
				c.RunCommand(context.Background(), "sp", &apiclient.RunCommandRequest{Command: "ls", Args: []string{"-l"}, Timeout: 5, Workdir: "/w"})
			},
			&api.RunCommandRequest{Command: "ls", Args: []string{"-l"}, Timeout: 5, Workdir: "/w"}},
		{"find", "/files/find", func() any { return &msg.FindMessage{} },
			func(c *apiclient.ApiClient) {
				c.Find(context.Background(), "sp", apiclient.FindRequest{Path: "/d", Recursive: true, Type: "any", IncludeHidden: true, IncludeMetadata: true, IncludeHash: true, IncludeSymlinks: true, MtimeMin: &mtimeMin})
			},
			nil},
		{"grep", "/files/grep", func() any { return &msg.GrepMessage{} },
			func(c *apiclient.ApiClient) {
				c.Grep(context.Background(), "sp", apiclient.GrepRequest{Pattern: "p", Path: "/d", Recursive: true, IgnoreCase: true})
			},
			&msg.GrepMessage{Pattern: "p", Path: "/d", Recursive: true, IgnoreCase: true}},
		{"edit", "/files/edit", func() any { return &msg.EditFileMessage{} },
			func(c *apiclient.ApiClient) {
				c.EditFile(context.Background(), "sp", apiclient.EditFileRequest{Path: "/e", Search: "a", Replace: "b"})
			},
			&msg.EditFileMessage{Path: "/e", Search: "a", Replace: "b"}},
		{"delete", "/files/delete", func() any { return &msg.DeleteFileMessage{} },
			func(c *apiclient.ApiClient) {
				c.DeleteSpaceFile(context.Background(), "sp", apiclient.DeleteFileRequest{Path: "/x", Recursive: true})
			},
			&msg.DeleteFileMessage{Path: "/x", Recursive: true}},
	}

	for _, contentType := range []string{rest.ContentTypeMsgPack, rest.ContentTypeJSON} {
		for _, tc := range calls {
			var got any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set(build.VersionHeader, build.Version)
				if !strings.HasSuffix(r.URL.Path, tc.path) {
					t.Errorf("%s: request to %s, want …%s", tc.name, r.URL.Path, tc.path)
				}
				got = tc.into()
				if err := rest.DecodeRequestBody(w, r, got); err != nil {
					t.Errorf("%s (%s): decode: %v", tc.name, contentType, err)
					return
				}
				rest.WriteResponse(200, w, r, map[string]any{"success": true})
			}))
			c, err := apiclient.NewClient(server.URL, "token", true)
			if err != nil {
				t.Fatal(err)
			}
			c.SetContentType(contentType)
			tc.send(c)
			server.Close()

			if tc.expect == nil { // find: check the fields mirror relies on
				f, _ := got.(*msg.FindMessage)
				if f == nil || f.Path != "/d" || !f.Recursive || f.Type != "any" || !f.IncludeHidden || !f.IncludeMetadata ||
					!f.IncludeHash || !f.IncludeSymlinks || f.MtimeMin == nil || *f.MtimeMin != mtimeMin {
					t.Errorf("find (%s): server decoded %+v", contentType, got)
				}
				continue
			}
			if !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("%s (%s): server decoded %+v, want %+v", tc.name, contentType, got, tc.expect)
			}
		}
	}
}
