package agentcmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/rs/zerolog/log"
)

func fetchCodeServer() error {
	// Get the host architecture, arm64 or amd64
	var arch string
	switch runtime.GOARCH {
	case "amd64":
		arch = "amd64"
	case "arm64":
		arch = "arm64"
	default:
		return fmt.Errorf("unsupported architecture: '%s'", runtime.GOARCH)
	}

	// Create ~/.local/bin and ~/.local/lib directories if they don't exist
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".local", "bin")); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Join(os.Getenv("HOME"), ".local", "bin"), 0755); err != nil {
			return fmt.Errorf("failed to create directory: %v", err)
		}
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".local", "lib")); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Join(os.Getenv("HOME"), ".local", "lib"), 0755); err != nil {
			return fmt.Errorf("failed to create directory: %v", err)
		}
	}

	// Get the latest version of code-server
	log.Info().Msg("code-server: checking the latest version of code-server..")
	resp, err := http.Get("https://api.github.com/repos/coder/code-server/releases/latest")
	if err != nil {
		return fmt.Errorf("failed to get latest version: %v", err)
	}
	defer resp.Body.Close()

	// Decode the JSON response and get latest version from the tag_name field
	type GitHubRelease struct {
		TagName string `json:"tag_name"`
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return fmt.Errorf("failed to decode JSON response: %v", err)
	}
	latestVersion := release.TagName
	latestVersion = strings.Trim(latestVersion, "\",v ")
	log.Info().Msgf("code-server: latest version of code-server is %s", latestVersion)

	// Check if the latest version is already installed
	codeServerPath := filepath.Join(os.Getenv("HOME"), ".local", "lib", "code-server-"+latestVersion)
	if _, err := os.Stat(codeServerPath); !os.IsNotExist(err) {
		log.Error().Msgf("code-server %s is already installed", latestVersion)
		return nil
	}

	// Download the latest version of code-server
	log.Info().Msg("code-server: downloading code-server..")
	downloadURL := fmt.Sprintf("https://github.com/coder/code-server/releases/download/v%s/code-server-%s-linux-%s.tar.gz", latestVersion, latestVersion, arch)
	err = downloadUnpackTgz(downloadURL, filepath.Join(os.Getenv("HOME"), ".local", "lib"))
	if err != nil {
		return fmt.Errorf("failed to download code-server: %v", err)
	}

	// Move the code-server to the correct directory
	log.Info().Msg("code-server: installing code-server..")
	if err := os.Rename(filepath.Join(os.Getenv("HOME"), ".local", "lib", "code-server-"+latestVersion+"-linux-"+arch), codeServerPath); err != nil {
		return fmt.Errorf("failed to move code-server: %v", err)
	}
	if err := os.Symlink(filepath.Join(codeServerPath, "bin", "code-server"), filepath.Join(os.Getenv("HOME"), ".local", "bin", "code-server")); err != nil {
		return fmt.Errorf("failed to create symlink: %v", err)
	}

	// Remove old versions of code-server
	files, err := os.ReadDir(filepath.Join(os.Getenv("HOME"), ".local", "lib"))
	if err != nil {
		return fmt.Errorf("failed to read directory: %v", err)
	}
	for _, file := range files {
		if strings.HasPrefix(file.Name(), "code-server-") && file.Name() != "code-server-"+latestVersion {
			log.Printf("Removing old version: %s\n", file.Name())
			if err := os.RemoveAll(filepath.Join(os.Getenv("HOME"), ".local", "lib", file.Name())); err != nil {
				return fmt.Errorf("failed to remove old version: %v", err)
			}
		}
	}

	log.Info().Msgf("code-server: %s installed successfully", latestVersion)
	return nil
}

func startCodeServer(port int) {
	if port > 0 {
		// Fetch the latest version of code-server
		if err := fetchCodeServer(); err != nil {
			log.Error().Msgf("code-server: %v", err)
			return
		}

		// Start code-server
		log.Info().Msg("code-server: starting...")
		cmd := exec.Command(filepath.Join(os.Getenv("HOME"), ".local", "bin", "code-server"), "--disable-telemetry", "--auth", "none", "--bind-addr", fmt.Sprintf("127.0.0.1:%d", port))

		// Redirect output to syslog
		redirectToSyslog(cmd)

		if err := cmd.Start(); err != nil {
			log.Error().Msgf("code-server: error starting: %v", err)
			return
		}

		// Run code-server in the background
		go func() {
			if err := cmd.Wait(); err != nil {
				log.Error().Msgf("code-server: exited with error: %v", err)
			}
		}()
	}
}
