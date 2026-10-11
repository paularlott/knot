package filestore

import (
	"os"
	"testing"
)

// Most tests check that content goes with its last file, so content is not
// held for reuse unless a test asks for it.
func TestMain(m *testing.M) {
	contentGrace = 0
	os.Exit(m.Run())
}
