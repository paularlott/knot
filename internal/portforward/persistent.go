package portforward

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/paularlott/knot/internal/log"
)

// ForwardEntry represents a single persistent forward in the TOML file.
type ForwardEntry struct {
	LocalPort  uint16 `toml:"local_port"`
	Space      string `toml:"space"`
	RemotePort uint16 `toml:"remote_port"`
}

type portForwardConfig struct {
	Forwards []ForwardEntry `toml:"forward"`
}

const knotDir = ".knot"

// testConfigPath overrides the config file path in tests.
var testConfigPath *string

// configFilePath returns the path to ~/.knot/port-forward.toml
func configFilePath() (string, error) {
	if testConfigPath != nil {
		return *testConfigPath, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return home + "/" + knotDir + "/port-forward.toml", nil
}

func loadConfig(path string) (portForwardConfig, error) {
	var cfg portForwardConfig
	_, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return portForwardConfig{}, nil
		}
		return portForwardConfig{}, err
	}
	return cfg, nil
}

func saveConfig(path string, cfg portForwardConfig) error {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(cfg); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0600)
}

// SaveForward persists a port forward entry to the TOML file.
// If an entry with the same local_port already exists, it is replaced.
func SaveForward(localPort, remotePort uint16, space string) error {
	path, err := configFilePath()
	if err != nil {
		return err
	}

	cfg, err := loadConfig(path)
	if err != nil {
		return err
	}

	found := false
	for i, e := range cfg.Forwards {
		if e.LocalPort == localPort {
			cfg.Forwards[i] = ForwardEntry{LocalPort: localPort, Space: space, RemotePort: remotePort}
			found = true
			break
		}
	}
	if !found {
		cfg.Forwards = append(cfg.Forwards, ForwardEntry{LocalPort: localPort, Space: space, RemotePort: remotePort})
	}

	return saveConfig(path, cfg)
}

// RemoveForward removes the forward entry matching localPort from the TOML file.
// No-op if the entry doesn't exist or the file doesn't exist.
func RemoveForward(localPort uint16) error {
	path, err := configFilePath()
	if err != nil {
		return err
	}

	cfg, err := loadConfig(path)
	if err != nil {
		return err
	}

	filtered := cfg.Forwards[:0]
	for _, e := range cfg.Forwards {
		if e.LocalPort != localPort {
			filtered = append(filtered, e)
		}
	}
	cfg.Forwards = filtered

	return saveConfig(path, cfg)
}

// LoadForwards reads all forward entries from the TOML file.
// Returns empty slice if the file doesn't exist.
func LoadForwards() ([]ForwardEntry, error) {
	path, err := configFilePath()
	if err != nil {
		return nil, err
	}
	cfg, err := loadConfig(path)
	return cfg.Forwards, err
}

// IsPersistent checks if a forward with the given localPort is persisted in the TOML file.
func IsPersistent(localPort uint16) bool {
	entries, err := LoadForwards()
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.LocalPort == localPort {
			return true
		}
	}
	return false
}

// RestoreForwardFunc is set by the agent startup to break the import cycle.
var RestoreForwardFunc func(entry ForwardEntry) error

// WaitForCredentials is set by the agent startup to block restore until the
// agent has registered with the server and has a valid token/URL.
// Returns true if credentials are ready, false if timed out.
var WaitForCredentials func(timeout time.Duration) bool

// RestoreForwards loads all persistent forwards and restores them via the command socket.
// It waits up to 30 seconds for the agent to have credentials before attempting restore.
func RestoreForwards() {
	entries, err := LoadForwards()
	if err != nil {
		log.WithError(err).Error("Failed to load persistent forwards")
		return
	}

	if len(entries) == 0 {
		return
	}

	if RestoreForwardFunc == nil {
		log.Error("RestoreForwardFunc not set, cannot restore forwards")
		return
	}

	if WaitForCredentials != nil {
		if !WaitForCredentials(30 * time.Second) {
			log.Error("Timed out waiting for agent credentials, cannot restore forwards")
			return
		}
	}

	for _, entry := range entries {
		if err := RestoreForwardFunc(entry); err != nil {
			log.Error("Failed to restore forward", "local_port", entry.LocalPort, "error", err)
		} else {
			log.Info("Restored persistent forward", "local_port", entry.LocalPort, "space", entry.Space, "remote_port", entry.RemotePort)
		}
	}
}
