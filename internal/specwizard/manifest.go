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
	Name          string   `toml:"name" json:"name" msgpack:"name"`
	DisplayName   string   `toml:"display_name" json:"display_name" msgpack:"display_name"`
	Description   string   `toml:"description" json:"description" msgpack:"description"`
	Image         string   `toml:"image" json:"image" msgpack:"image"`
	Icon          string   `toml:"icon" json:"icon" msgpack:"icon"`
	Category      string   `toml:"category" json:"category" msgpack:"category"`
	Tags          []string `toml:"tags" json:"tags" msgpack:"tags"`
	DefaultMemory string   `toml:"default_memory" json:"default_memory" msgpack:"default_memory"`
	DefaultCPUs   string   `toml:"default_cpus" json:"default_cpus" msgpack:"default_cpus"`
	DefaultCores  string   `toml:"default_cores" json:"default_cores" msgpack:"default_cores"`
	Recommended   bool     `toml:"recommended" json:"recommended" msgpack:"recommended"`

	// DefaultEnv are KEY=value env vars pre-filled when picking this image
	// (e.g. KNOT_VNC_HTTP_PORT=5680 for desktop images).
	DefaultEnv []string `toml:"default_env" json:"default_env,omitempty" msgpack:"default_env,omitempty"`

	// DefaultPorts are template-level ports pre-filled when picking this
	// image (e.g. Web:80:http for PHP images). These are the template port
	// metadata, not the Nomad/Docker network ports.
	DefaultPorts []ManifestPort `toml:"default_port" json:"default_port,omitempty" msgpack:"default_port,omitempty"`

	// Volumes are mount points the image expects to be backed by persistent
	// storage (e.g. /home for Ubuntu/PHP, /data for Valkey/Redis,
	// /var/lib/mysql for MariaDB). The wizard pre-fills one storage row per
	// entry when the user picks this image. Only the mount point is declared
	// here; the backing kind (named volume / managed path / bind) is chosen in
	// the wizard.
	Volumes []ManifestVolume `toml:"volume" json:"volumes,omitempty" msgpack:"volumes,omitempty"`
}

// ManifestPort is a template-level port entry in the manifest.
type ManifestPort struct {
	Name     string `toml:"name" json:"name" msgpack:"name"`
	Port     uint16 `toml:"port" json:"port" msgpack:"port"`
	Protocol string `toml:"protocol" json:"protocol" msgpack:"protocol"`
}

// ManifestVolume is a mount point within the container that the image expects
// to be persistent. Path is the in-container mount point (e.g. /data).
type ManifestVolume struct {
	Path string `toml:"path" json:"path" msgpack:"path"`
	// Kind is the suggested backing kind for the wizard's storage row:
	// "bind", "volume", or "path". Empty (the default) means "volume". Invalid
	// values are normalised to empty by the parser. The user can still change
	// the kind in the wizard after picking the image.
	Kind        string `toml:"kind" json:"kind,omitempty" msgpack:"kind,omitempty"`
	Description string `toml:"description" json:"description,omitempty" msgpack:"description,omitempty"`
}

// Manifest is the parsed catalog of base images plus catalog metadata.
type Manifest struct {
	// Version is the manifest schema version (currently 1). Bumped only when
	// the structure of this file changes in a backward-incompatible way.
	Version int `toml:"version" json:"version" msgpack:"version"`

	// ManifestVersion is the catalog revision, used to decide whether a
	// fetched manifest is newer than the built-in one. Format yyyymmddbb
	// (date + zero-padded same-day build counter); lexicographic compare
	// equals chronological compare. Empty means "unset / oldest".
	ManifestVersion string `toml:"manifest_version" json:"manifest_version,omitempty" msgpack:"manifest_version,omitempty"`

	Description string       `toml:"description" json:"description" msgpack:"description"`
	Images      []ImageEntry `toml:"image" json:"images" msgpack:"images"`

	// RegistryAuth is set by the API handler (not parsed from TOML) to tell
	// the wizard whether server.base_image.registry_user/password are
	// configured, so it can inject an auth block when picking an image.
	RegistryAuth bool `toml:"-" json:"registry_auth" msgpack:"-"`
}

var (
	storeMu sync.RWMutex

	// embeddedManifest is the cached parse of the bundled manifest.toml.
	// It never changes after first parse, so it is cached for the process.
	embeddedManifest *Manifest

	// fetchedManifest holds a manifest downloaded from the update URL (or
	// received over gossip). It is only set when its ManifestVersion is
	// strictly newer than the embedded baseline. Nil when nothing has been
	// fetched or the fetched copy was older than the embedded one.
	fetchedManifest *Manifest

	// fetchedSource records where fetchedManifest came from ("fetch",
	// "gossip", "api") for logging/debug.
	fetchedSource string
)

// parseManifest parses raw TOML bytes into a normalised Manifest.
func parseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := toml.Unmarshal(data, &m); err != nil {
		return nil, err
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
		for j := range e.Volumes {
			v := &m.Images[i].Volumes[j]
			v.Path = strings.TrimSpace(v.Path)
			switch v.Kind {
			case "bind", "volume", "path":
				// valid backing kind; keep as-is
			default:
				// empty (default → volume) or unrecognised → treat as unset
				v.Kind = ""
			}
		}
	}

	return &m, nil
}

// loadEmbedded returns the cached parse of the bundled manifest.
func loadEmbedded() *Manifest {
	storeMu.Lock()
	defer storeMu.Unlock()
	if embeddedManifest == nil {
		// A parse failure here is a build-time bug; panic so it surfaces
		// immediately rather than degrading silently.
		m, err := parseManifest(defaultManifest)
		if err != nil {
			panic(fmt.Errorf("parse embedded base images manifest: %w", err))
		}
		embeddedManifest = m
	}
	return embeddedManifest
}

// loadExternal reads and parses an on-disk manifest file. It is re-read on
// every call so admins can update the file and have the next request pick up
// the changes without restarting the server.
func loadExternal(path string) (*Manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read base images manifest %q: %w", path, err)
	}
	m, err := parseManifest(raw)
	if err != nil {
		return nil, fmt.Errorf("parse base images manifest: %w", err)
	}
	return m, nil
}

// versionGreater reports whether a > b under the manifest_version ordering.
// Versions are yyyymmddbb strings, so lexicographic compare is chronological.
// An empty version is treated as "oldest" — it never beats anything, and a
// fetch with no version is never stored.
func versionGreater(a, b string) bool {
	return a != "" && a > b
}

// baselineManifest returns the non-fetched baseline catalog: the external file
// (re-read) if configured and readable, otherwise the embedded catalog. A read
// error on the external file falls back to the embedded baseline so a transient
// file failure doesn't block a fetch/compare (LoadManifest still surfaces the
// error when serving).
func baselineManifest() *Manifest {
	cfg := config.GetServerConfig()
	if cfg != nil && cfg.BaseImagesManifest != "" {
		if m, err := loadExternal(cfg.BaseImagesManifest); err == nil {
			return m
		}
	}
	return loadEmbedded()
}

// LoadManifest returns the active manifest. The model is "newest wins" between
// a baseline and an optional fetched overlay:
//
//  1. Baseline = the external file (cfg.BaseImagesManifest), re-read on every
//     call so admins can update the file in place; otherwise the embedded
//     default.
//  2. If a manifest fetched from the update URL is in memory AND its
//     manifest_version is strictly newer than the baseline, it overlays the
//     baseline. A fetched copy older than or equal to the baseline is ignored.
//
// So with a file configured, the file is served as-is unless auto-update has
// fetched a newer remote copy — in which case the remote wins. If the admin
// bumps the file's version above the fetched one, the file wins again on the
// next request (the file is re-read every call).
func LoadManifest(cfg *config.ServerConfig) (*Manifest, error) {
	path := ""
	if cfg != nil {
		path = cfg.BaseImagesManifest
	}

	var baseline *Manifest
	if path != "" {
		b, err := loadExternal(path)
		if err != nil {
			return nil, err
		}
		baseline = b
	} else {
		baseline = loadEmbedded()
	}

	storeMu.RLock()
	f := fetchedManifest
	storeMu.RUnlock()
	if f != nil && versionGreater(f.ManifestVersion, baseline.ManifestVersion) {
		return f, nil
	}
	return baseline, nil
}

// EmbeddedManifestVersion returns the catalog revision compiled into the
// binary.
func EmbeddedManifestVersion() string {
	return loadEmbedded().ManifestVersion
}

// ActiveManifestVersion returns the revision of the manifest LoadManifest would
// currently return: the newer of the baseline (external file if configured,
// else embedded) and any fetched overlay.
func ActiveManifestVersion() string {
	baselineVer := baselineManifest().ManifestVersion
	storeMu.RLock()
	fv := ""
	if fetchedManifest != nil {
		fv = fetchedManifest.ManifestVersion
	}
	storeMu.RUnlock()
	if versionGreater(fv, baselineVer) {
		return fv
	}
	return baselineVer
}

// SetFetchedManifest parses a downloaded manifest body and stores it as the
// fetched overlay iff its manifest_version is strictly newer than both the
// current baseline (external file if configured, else embedded) and any
// previously stored fetch. Returns (true, nil) when stored, (false, nil) when
// ignored as not newer.
//
// With a file configured, this is exactly "check if the remote is newer than
// the local file and use the remote; otherwise keep the local". The file is
// re-read here so a freshly-bumped file version is respected.
func SetFetchedManifest(data []byte, source string) (bool, error) {
	incoming, err := parseManifest(data)
	if err != nil {
		return false, fmt.Errorf("parse fetched base images manifest: %w", err)
	}

	// Read the baseline version before taking the lock (it may read a file).
	baselineVer := baselineManifest().ManifestVersion

	storeMu.Lock()
	defer storeMu.Unlock()

	threshold := baselineVer
	if fetchedManifest != nil && versionGreater(fetchedManifest.ManifestVersion, threshold) {
		threshold = fetchedManifest.ManifestVersion
	}

	if !versionGreater(incoming.ManifestVersion, threshold) {
		return false, nil
	}

	fetchedManifest = incoming
	fetchedSource = source
	return true, nil
}

// FetchedManifestSource returns where the active fetched manifest came from
// ("startup", "api"), or "" when the embedded/external manifest is active.
func FetchedManifestSource() string {
	storeMu.RLock()
	defer storeMu.RUnlock()
	return fetchedSource
}

// ResetFetched clears any in-memory fetched manifest so the embedded default
// is used again. Mainly for tests.
func ResetFetched() {
	storeMu.Lock()
	fetchedManifest = nil
	fetchedSource = ""
	storeMu.Unlock()
}

// Reload is retained for compatibility; it clears the embedded parse cache so
// the next LoadManifest re-parses. (External files are never cached.)
func Reload() {
	storeMu.Lock()
	embeddedManifest = nil
	fetchedManifest = nil
	fetchedSource = ""
	storeMu.Unlock()
}
