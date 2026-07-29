package specwizard

import (
	_ "embed"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
	"github.com/paularlott/knot/internal/config"
)

//go:embed manifest.toml
var defaultManifest []byte

// ImageEntry describes a single base image in the wizard catalog. The Image
// field may contain template variables (e.g. ${{ .server.base_image_registry }});
// those are resolved at render time by the wizard caller, not by the loader.
type ImageEntry struct {
	Name          string   `toml:"name" json:"name"`
	DisplayName   string   `toml:"display_name" json:"display_name"`
	Description   string   `toml:"description" json:"description"`
	Image         string   `toml:"image" json:"image"`
	Icon          string   `toml:"icon" json:"icon"`
	Category      string   `toml:"category" json:"category"`
	Tags          []string `toml:"tags" json:"tags"`
	DefaultMemory string   `toml:"default_memory" json:"default_memory"`
	DefaultCPUs   string   `toml:"default_cpus" json:"default_cpus"`
	DefaultCores  string   `toml:"default_cores" json:"default_cores"`
	Recommended   bool     `toml:"recommended" json:"recommended"`
}

// Manifest is the parsed catalog of base images plus catalog metadata.
type Manifest struct {
	Version     int          `toml:"version" json:"version"`
	Description string       `toml:"description" json:"description"`
	Images      []ImageEntry `toml:"image" json:"images"`
}

var (
	cachedManifest *Manifest
	cachedPath     string
	cacheMu        sync.Mutex
)

// LoadManifest returns the active manifest. If server config names an external
// path (BaseImagesManifest), that file is loaded; otherwise the embedded
// default is used. The result is cached keyed by path so repeated calls are
// cheap. An empty cfg yields the embedded manifest.
func LoadManifest(cfg *config.ServerConfig) (*Manifest, error) {
	path := ""
	if cfg != nil {
		path = cfg.BaseImagesManifest
	}

	cacheMu.Lock()
	defer cacheMu.Unlock()

	if cachedManifest != nil && cachedPath == path {
		return cachedManifest, nil
	}

	var data []byte
	if path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read base images manifest %q: %w", path, err)
		}
		data = raw
	} else {
		data = defaultManifest
	}

	var m Manifest
	if err := toml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse base images manifest: %w", err)
	}

	// Normalise: trim whitespace, default category.
	for i := range m.Images {
		e := &m.Images[i]
		e.Name = strings.TrimSpace(e.Name)
		e.DisplayName = strings.TrimSpace(e.DisplayName)
		e.Image = strings.TrimSpace(e.Image)
		if e.Category == "" {
			e.Category = "general"
		}
	}

	cachedManifest = &m
	cachedPath = path
	return &m, nil
}

// Reload forces the next LoadManifest call to re-read from disk. Used by tests
// and by an admin "refresh" path if added later.
func Reload() {
	cacheMu.Lock()
	cachedManifest = nil
	cachedPath = ""
	cacheMu.Unlock()
}
