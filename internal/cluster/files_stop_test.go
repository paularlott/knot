package cluster

import "testing"

// Stop can be called more than once, as on a shutdown signal arriving twice.
func TestStopFilesSyncTwice(t *testing.T) {
	c := &Cluster{}
	c.stopFilesSync()
	c.stopFilesSync()
	select {
	case <-filesSyncStop:
	default:
		t.Error("files sync not told to stop")
	}
}
