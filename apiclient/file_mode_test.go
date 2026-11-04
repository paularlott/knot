package apiclient_test

import (
	"os"
	"testing"

	"github.com/paularlott/knot/apiclient"
)

// Modes are kept as s3fs keeps them, the decimal st_mode, so a bucket
// mounted with s3fs or over knot's NFS server shows the same modes.
func TestFileModeRoundTrip(t *testing.T) {
	for _, c := range []struct {
		mode os.FileMode
		meta string
	}{
		{0644, "33188"},                  // 0100644
		{0755, "33261"},                  // 0100755
		{0600, "33152"},                  // 0100600
		{os.ModeDir | 0755, "16877"},     // 040755
		{os.ModeSymlink | 0777, "41471"}, // 0120777
		{os.ModeSetuid | 0755, "33261"},  // only the permission bits
	} {
		got := apiclient.FormatFileMode(c.mode)
		if got != c.meta {
			t.Errorf("FormatFileMode(%v) = %s, want %s", c.mode, got, c.meta)
		}
		mode, ok := apiclient.ParseFileMode(map[string]string{apiclient.FileModeMeta: got})
		want := c.mode & (os.ModePerm | os.ModeSymlink)
		if !ok || mode != want {
			t.Errorf("ParseFileMode(%s) = %v, %v, want %v", got, mode, ok, want)
		}
	}

	for _, meta := range []map[string]string{nil, {}, {apiclient.FileModeMeta: ""}, {apiclient.FileModeMeta: "rwx"}} {
		if _, ok := apiclient.ParseFileMode(meta); ok {
			t.Errorf("ParseFileMode(%v) found a mode", meta)
		}
	}
}
