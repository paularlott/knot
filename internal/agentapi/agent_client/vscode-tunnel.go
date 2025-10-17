package agent_client

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/paularlott/knot/internal/util"

	"github.com/paularlott/knot/internal/log"
)

func fetchVSCode() error {
	logger := log.WithGroup("vscode")

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %v", err)
	}

	// Test if code is already installed
	if _, err := os.Stat(filepath.Join(home, ".local", "bin", "code")); !os.IsNotExist(err) {
		logger.Info("Visual Studio Code is already installed")
		return nil
	}

	// Create ~/.local/bin if missing
	if _, err := os.Stat(filepath.Join(home, ".local", "bin")); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Join(home, ".local", "bin"), 0755); err != nil {
			return fmt.Errorf("failed to create directory: %v", err)
		}
	}

	// Get the host architecture, arm64 or amd64
	var arch string
	switch runtime.GOARCH {
	case "amd64":
		arch = "x64"
	case "arm64":
		arch = "arm64"
	default:
		return fmt.Errorf("unsupported architecture: '%s'", runtime.GOARCH)
	}

	logger.Info("downloading Visual Studio Code..")
	err = util.DownloadUnpackTgz(
		"https://code.visualstudio.com/sha/download?build=stable&os=cli-alpine-"+arch,
		filepath.Join(home, ".local", "bin"),
	)
	if err != nil {
		return fmt.Errorf("failed to download vscode: %v", err)
	}

	logger.Info("Visual Studio Code installed")

	return nil
}

func startVSCodeTunnel(name string) {
	logger := log.WithGroup("vscode")

	if name == "" {
		return
	}

	if err := fetchVSCode(); err != nil {
		logger.WithError(err).Error("error fetching vscode")
		return
	}

	home, err := os.UserHomeDir()
	if err != nil {
		logger.WithError(err).Error("failed to get user home directory")
		return
	}

	codeBin := filepath.Join(home, ".local", "bin", "code")

	// Start code-server
	logger.Info("starting...")
	cmd := exec.Command(
		"screen",
		"-dmS",
		name,
		"bash",
		"-c",
		"while true; do "+codeBin+" tunnel --accept-server-license-terms; sleep 1; done",
	)

	// Redirect output to syslog
	util.RedirectToSyslog(cmd)

	if err := cmd.Start(); err != nil {
		logger.WithError(err).Error("error starting:")
		return
	}

	// Run code-server in the background
	go func() {
		if err := cmd.Wait(); err != nil {
			logger.WithError(err).Error("exited with error:")
		}
	}()
}
