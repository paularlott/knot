package agent_client

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/paularlott/knot/internal/util"

	"github.com/rs/zerolog/log"
)

func fetchVSCode() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %v", err)
	}

	// Test if code is already installed
	if _, err := os.Stat(filepath.Join(home, ".local", "bin", "code")); !os.IsNotExist(err) {
		log.Info().Msg("vscode: Visual Studio Code is already installed")
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

	log.Info().Msg("vscode: downloading Visual Studio Code..")
	err = util.DownloadUnpackTgz(
		"https://code.visualstudio.com/sha/download?build=stable&os=cli-alpine-"+arch,
		filepath.Join(home, ".local", "bin"),
	)
	if err != nil {
		return fmt.Errorf("failed to download vscode: %v", err)
	}

	log.Info().Msg("vscode: Visual Studio Code installed")

	return nil
}

func startVSCodeTunnel(name string) {
	if name == "" {
		return
	}

	if err := fetchVSCode(); err != nil {
		log.Error().Msgf("vscode: %v", err)
		return
	}

	home, err := os.UserHomeDir()
	if err != nil {
		log.Error().Msgf("vscode: failed to get user home directory %v", err)
		return
	}

	codeBin := filepath.Join(home, ".local", "bin", "code")

	// Start code-server
	log.Info().Msg("vscode: starting...")
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
		log.Error().Msgf("vscode: error starting: %v", err)
		return
	}

	// Run code-server in the background
	go func() {
		if err := cmd.Wait(); err != nil {
			log.Error().Msgf("vscode: exited with error: %v", err)
		}
	}()
}
