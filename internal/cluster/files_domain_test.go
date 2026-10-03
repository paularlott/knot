package cluster

import (
	"testing"

	"github.com/paularlott/knot/internal/config"
)

// Origin servers keep advertising the value they always have, so they stay
// compatible with older servers; leaf nodes advertise their own domain and so
// never replicate files with origin servers.
func TestLocalFilesDomain(t *testing.T) {
	prev := config.GetServerConfig()
	t.Cleanup(func() { config.SetServerConfig(prev) })

	config.SetServerConfig(&config.ServerConfig{})
	if d := localFilesDomain(); d != "1" {
		t.Errorf("origin domain %q", d)
	}
	config.SetServerConfig(&config.ServerConfig{LeafNode: true})
	if d := localFilesDomain(); d != filesDomainLeaf {
		t.Errorf("leaf domain %q", d)
	}
	var c Cluster
	if c.isFilesNode(nil) {
		t.Error("nil node counted as a files node")
	}
}
