package harness

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// buildDir is where test artifacts (logs, reports) are written.
const buildDir = "build/test"

// Binaries holds the path of the built server binary.
type Binaries struct {
	// Server is the knot binary built by `task build` (bin/knot). That task
	// rebuilds the agent zips into web/agents/ first, so the agents served
	// from /agents/ are embedded by go:embed and always match the server
	// build — the agent handshake enforces major.minor equality.
	Server string
}

// BuildBinaries runs `task build`, which compiles the server for the host
// platform with freshly built embedded agents, web assets and legal files.
// It uses the repo's own Taskfile, so the pro fork automatically builds with
// its pro build flags. Task's source checksums make repeat runs incremental.
func BuildBinaries() (*Binaries, error) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		return nil, err
	}

	cmd := exec.Command("task", "build")
	cmd.Dir = repoRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("task build: %w", err)
	}

	serverBin := filepath.Join(repoRoot, "bin", "knot")
	if _, err := os.Stat(serverBin); err != nil {
		return nil, fmt.Errorf("task build did not produce %s: %w", serverBin, err)
	}

	return &Binaries{Server: serverBin}, nil
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if strings.HasSuffix(filepath.ToSlash(dir), "integration-tests/harness") ||
				strings.HasSuffix(filepath.ToSlash(dir), "integration-tests/suites") {
				dir = filepath.Dir(filepath.Dir(dir))
				continue
			}
			return dir, nil
		}
		dir = filepath.Dir(dir)
	}
	return "", fmt.Errorf("could not find repo root above %s", dir)
}
