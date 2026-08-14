package configwizard

import (
	"strings"
	"testing"
)

const mergeExisting = `# my knot config — hand edited

[server] # main section
listen = "127.0.0.1:3000"
custom_flag = true  # mine

[server.dns]
enabled = true
listen = "192.168.1.1:3053"
records = [
  "A|knot.internal|192.168.1.1|300",
]

[my.custom.section]
key = "keep me"

[server.chat]
enabled = true
model = "llama3.2"
`

const mergeGenerated = `[server]
listen = "0.0.0.0:3000"
url = "http://x/"

[server.dns]
enabled = false

[server.badgerdb]
enabled = true
path = "/data"
`

func TestMergeConfig(t *testing.T) {
	merged := mergeConfig(mergeExisting, mergeGenerated)

	checks := []struct {
		name string
		want string
	}{
		{"top comment preserved", "# my knot config — hand edited"},
		{"unknown key in managed table preserved", "custom_flag = true"},
		{"wizard value wins for managed key", `listen = "0.0.0.0:3000"`},
		{"wizard-only key added", `url = "http://x/"`},
		{"wizard managed table overridden", "enabled = false"},
		{"multiline array in unmanaged part preserved", `"A|knot.internal|192.168.1.1|300",`},
		{"unknown section preserved", "[my.custom.section]"},
		{"unmanaged table kept verbatim", `[server.chat]`},
		{"unmanaged table keys kept", `model = "llama3.2"`},
		{"new table appended", "[server.badgerdb]"},
	}
	for _, c := range checks {
		if !strings.Contains(merged, c.want) {
			t.Errorf("%s: merged output missing %q\nmerged:\n%s", c.name, c.want, merged)
		}
	}

	// The old wizard-managed listen value must not survive alongside the new one.
	if strings.Contains(merged, `listen = "127.0.0.1:3000"`) {
		t.Errorf("stale managed key survived merge:\n%s", merged)
	}

	// The merged document must have exactly one [server] block.
	if n := strings.Count(merged, "[server]"); n != 1 {
		t.Errorf("expected 1 [server] header, got %d\n%s", n, merged)
	}
}

func TestMergeConfigEmptyExisting(t *testing.T) {
	if got := mergeConfig("", mergeGenerated); got != mergeGenerated {
		t.Errorf("empty existing should return generated verbatim, got:\n%s", got)
	}
}
