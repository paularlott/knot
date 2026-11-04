package apiclient_test

import (
	"os"
	"testing"

	"github.com/paularlott/knot/apiclient"
)

// Modes are written as rclone --metadata writes them, the octal st_mode, so
// they round-trip through rclone.
func TestFileModeRoundTrip(t *testing.T) {
	for _, c := range []struct {
		mode os.FileMode
		meta string
	}{
		{0644, "100644"},
		{0755, "100755"},
		{0600, "100600"},
		{os.ModeDir | 0755, "40755"},
		{os.ModeSymlink | 0777, "120777"},
		{os.ModeSetuid | 0755, "100755"}, // only the permission bits
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
}

// Octal forms other tools write are read too; anything that is not a file's,
// folder's or link's mode is not, including the decimal form knot wrote
// before 0.42 (fsck rewrites it), so it is never misread.
func TestParseFileModeForms(t *testing.T) {
	for _, c := range []struct {
		meta string
		want os.FileMode
	}{
		{"100750", 0750},
		{"40700", 0700},   // a folder
		{"0100640", 0640}, // with a leading 0
		{"755", 0755},     // permission bits alone
		{" 100755 ", 0755},
	} {
		mode, ok := apiclient.ParseFileMode(map[string]string{apiclient.FileModeMeta: c.meta})
		if !ok || mode != c.want {
			t.Errorf("ParseFileMode(%q) = %v, %v, want %v", c.meta, mode, ok, c.want)
		}
	}

	for _, meta := range []map[string]string{
		nil, {}, {apiclient.FileModeMeta: ""}, {apiclient.FileModeMeta: "rwx"},
		{apiclient.FileModeMeta: "999999"}, {apiclient.FileModeMeta: "-1"},
		{apiclient.FileModeMeta: "33261"}, // decimal 0100755
		{apiclient.FileModeMeta: "33188"}, // decimal 0100644
		{apiclient.FileModeMeta: "16877"}, // decimal 040755
		{apiclient.FileModeMeta: "10644"}, // a FIFO
	} {
		if mode, ok := apiclient.ParseFileMode(meta); ok {
			t.Errorf("ParseFileMode(%v) found mode %v", meta, mode)
		}
	}
}
