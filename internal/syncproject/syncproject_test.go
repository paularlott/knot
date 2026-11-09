package syncproject

import (
	"slices"
	"strings"
	"testing"
)

func TestParseDefaults(t *testing.T) {
	p, err := Parse([]byte(`
[[sync]]
local = "~/src/foo"
remote = "src:projects/foo/"
`), "myproject")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "myproject" {
		t.Errorf("name = %q, want the file's", p.Name)
	}
	j := p.Sync[0]
	if j.Name != "foo" || j.SyncMode() != ModeTwoWay || !j.Watching() || !j.Deleting() {
		t.Errorf("defaults: name %q mode %q watch %v delete %v", j.Name, j.SyncMode(), j.Watching(), j.Deleting())
	}

	p, err = Parse([]byte(`
name = "named"
[[sync]]
local = "/a"
remote = "b1:x"
watch = false
delete = false
mode = "download"
`), "file")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "named" || p.Sync[0].Watching() || p.Sync[0].Deleting() || p.Sync[0].SyncMode() != ModeDownload {
		t.Errorf("settings not kept: %+v", p)
	}
}

func TestParseRefuses(t *testing.T) {
	for name, src := range map[string]string{
		"unknown setting":   "[[sync]]\nlocal = \"/a\"\nremote = \"b1:x\"\nwach = true\n",
		"no jobs":           "name = \"x\"\n",
		"no local":          "[[sync]]\nremote = \"b1:x\"\n",
		"no remote":         "[[sync]]\nlocal = \"/a\"\n",
		"remote not bucket": "[[sync]]\nlocal = \"/a\"\nremote = \"/tmp/x\"\n",
		"pattern":           "[[sync]]\nlocal = \"/a\"\nremote = \"b1:x/*\"\n",
		"bad mode":          "[[sync]]\nlocal = \"/a\"\nremote = \"b1:x\"\nmode = \"both\"\n",
		"same name":         "[[sync]]\nlocal = \"/a\"\nremote = \"b1:x\"\n[[sync]]\nlocal = \"/b/a\"\nremote = \"b1:y\"\n",
		"nested":            "[[sync]]\nname = \"a\"\nlocal = \"/a\"\nremote = \"b1:x\"\n[[sync]]\nname = \"b\"\nlocal = \"/a/b\"\nremote = \"b1:y\"\n",
		"slash in name":     "name = \"a/b\"\n[[sync]]\nlocal = \"/a\"\nremote = \"b1:x\"\n",
	} {
		if _, err := Parse([]byte(src), "p"); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestEncodeRoundTrips(t *testing.T) {
	no := false
	p := &Project{Name: "p", Exclude: []string{"x/"}, Sync: []Job{{Name: "j", Local: "/a", Remote: "b1:x/", Snapshot: "b1:t/", Watch: &no}}}
	data, err := p.Encode()
	if err != nil {
		t.Fatal(err)
	}
	back, err := Parse(data, "other")
	if err != nil {
		t.Fatal(err)
	}
	if back.Name != "p" || back.Sync[0].Snapshot != "b1:t/" || back.Sync[0].Watching() {
		t.Fatalf("round trip: %+v", back)
	}
}

func TestFromVolumes(t *testing.T) {
	p, err := FromVolumes(`
volumes:
  data:
sync:
  - local: /home/user/src
    remote: src:projects/foo/
    snapshot: src:templates/go/
    exclude: [node_modules/]
`)
	if err != nil {
		t.Fatal(err)
	}
	if p == nil || p.Name != TemplateProject || len(p.Sync) != 1 || p.Sync[0].Name != "src" || p.Sync[0].Exclude[0] != "node_modules/" {
		t.Fatalf("got %+v", p)
	}
	if p, err := FromVolumes("volumes:\n  data:\n"); err != nil || p != nil {
		t.Fatalf("volumes without sync: %+v, %v", p, err)
	}
	if _, err := FromVolumes("sync:\n  - local: /a\n"); err == nil {
		t.Fatal("a job without remote was accepted")
	}
}

func TestGitignorePatterns(t *testing.T) {
	patterns, skipped := GitignorePatterns([]byte("# comment\n\n/build/\n**/node_modules\n*.log\n!keep.log\nvendor/**\r\n"))
	want := []string{"build/", "node_modules", "*.log", "vendor"}
	if !slices.Equal(patterns, want) {
		t.Errorf("patterns = %q, want %q", patterns, want)
	}
	if strings.Join(skipped, ",") != "!keep.log" {
		t.Errorf("skipped = %q", skipped)
	}
}
