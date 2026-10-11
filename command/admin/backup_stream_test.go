package commands_admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/paularlott/knot/apiclient"
)

// A backup stream is complete only if it ends with its end line giving the
// number of records: a stream cut short, even at a record boundary, is refused.
func TestBackupKindNeedsEndLine(t *testing.T) {
	for name, tc := range map[string]struct {
		body string
		want int
		fail bool
	}{
		"complete":      {"{\"a\":1}\n{\"a\":2}\n{\"_end\":2}\n", 2, false},
		"empty":         {"{\"_end\":0}\n", 0, false},
		"cut at a line": {"{\"a\":1}\n{\"a\":2}\n", 0, true},
		"cut short":     {"{\"a\":1}\n", 0, true},
		"wrong count":   {"{\"a\":1}\n{\"_end\":2}\n", 0, true},
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			client, err := apiclient.NewClient(srv.URL, "tk_test", true)
			if err != nil {
				t.Fatal(err)
			}
			n, err := backupKind(context.Background(), client, t.TempDir(), "roles", "", url.Values{}, nil)
			if tc.fail {
				if err == nil || !strings.Contains(err.Error(), "ended early") {
					t.Fatalf("a cut stream was accepted: %d, %v", n, err)
				}
				return
			}
			if err != nil || n != tc.want {
				t.Fatalf("%d records, %v; want %d", n, err, tc.want)
			}
		})
	}
}
