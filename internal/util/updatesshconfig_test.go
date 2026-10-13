package util

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadSSHConfigAgentForwarding(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// No file yet
	hosts, err := ReadSSHConfigAgentForwarding("default")
	if err != nil || len(hosts) != 0 {
		t.Fatalf("no file: %v, %v", hosts, err)
	}

	os.MkdirAll(filepath.Join(home, ".ssh"), 0700)
	config := `Host other
  ForwardAgent yes
#===KNOT-START===
Host knot.on
  HostName knot.on
  ForwardAgent yes
Host knot.off
  HostName knot.off
Host knot.edited
  forwardagent=no
#===KNOT-END===
#===KNOT-START (work)===
Host knot.on.work
  ForwardAgent yes
#===KNOT-END (work)===
`
	if err := os.WriteFile(filepath.Join(home, ".ssh", "config"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}

	hosts, err = ReadSSHConfigAgentForwarding("default")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"knot.on": true, "knot.off": false, "knot.edited": false}
	if len(hosts) != len(want) {
		t.Fatalf("hosts = %v, want %v", hosts, want)
	}
	for host, forward := range want {
		if got, ok := hosts[host]; !ok || got != forward {
			t.Fatalf("hosts = %v, want %v", hosts, want)
		}
	}

	// Each alias has its own block
	hosts, err = ReadSSHConfigAgentForwarding("work")
	if err != nil || len(hosts) != 1 || !hosts["knot.on.work"] {
		t.Fatalf("work alias: %v, %v", hosts, err)
	}
}
