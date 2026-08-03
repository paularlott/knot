package specwizard

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/log"
)

const (
	fetchTimeout = 30 * time.Second

	// DefaultUpdateURL is the fallback manifest URL used when no explicit
	// --base-images-update-url is configured and no manifest file is in use.
	DefaultUpdateURL = "https://getknot.dev/base-images.toml"

	// Source tags recorded against a fetched manifest for diagnostics.
	SourceStartup = "startup"
	SourceAPI     = "api"
)

// FetchDecision decides whether a manifest fetch should happen and from what
// URL, based on the server configuration. The startup fetch and the manual
// refresh follow the SAME rule, governed by --base-images-update-enabled as a
// master gate: when it is off, no fetch happens at all (neither startup nor
// manual); when it is on, a fetch happens subject to the manifest/url rules.
//
// Notation: M = base-images-manifest set, A = base-images-update-enabled on,
// U = base-images-update-url set.
//
//	Fetch iff A && (!M || U)
//
// URL: the configured --base-images-update-url when set (U); otherwise the
// DefaultUpdateURL — but the default is only ever reached when no manifest file
// is configured (M=false), because the only M-set case that fetches requires U.
func FetchDecision(cfg *config.ServerConfig) (url string, ok bool) {
	if cfg == nil {
		return "", false
	}
	M := cfg.BaseImagesManifest != ""
	A := cfg.BaseImagesUpdateEnabled
	U := cfg.BaseImagesUpdateURL != ""

	if !(A && (!M || U)) {
		return "", false
	}
	if U {
		return cfg.BaseImagesUpdateURL, true
	}
	return DefaultUpdateURL, true
}

// FetchDisabledReason returns a human-readable explanation of why FetchDecision
// returned ok=false for the given config, for use in error messages.
func FetchDisabledReason(cfg *config.ServerConfig) string {
	if cfg == nil || !cfg.BaseImagesUpdateEnabled {
		return "base image updates are disabled on this server (set --base-images-update-enabled to enable)"
	}
	// A is on, so the only remaining no-fetch case is M && !U.
	return "a manifest file is in use; set --base-images-update-url to allow fetching a remote catalog"
}

// FetchOnStartup fetches the manifest once, in the background, per the startup
// FetchDecision. It returns immediately; success or failure is only logged.
// There is no periodic loop — the catalog only changes on startup or via an
// explicit refresh (RefreshNow / the admin CLI).
func FetchOnStartup() {
	cfg := config.GetServerConfig()
	url, ok := FetchDecision(cfg)
	if !ok {
		return
	}

	go func() {
		logger := log.WithGroup("base-images")
		stored, err := fetchAndStore(url, SourceStartup)
		switch {
		case err != nil:
			logger.Error("startup manifest fetch failed; using baseline catalog", "url", url, "error", err)
		case stored:
			logger.Info("startup manifest fetch updated the catalog", "version", ActiveManifestVersion())
		default:
			logger.Info("startup manifest fetch: baseline catalog already current", "version", ActiveManifestVersion())
		}
	}()
}

// RefreshNow forces an immediate fetch from url (the API / admin-CLI path) and
// stores the result as the fetched overlay iff it is newer than the baseline.
// Returns whether the fetched manifest became the active catalog. Callers use
// FetchDecision to decide whether to refresh at all and from which URL.
func RefreshNow(url string) (bool, error) {
	return fetchAndStore(url, SourceAPI)
}

// fetchAndStore GETs the manifest at url, feeds the body to SetFetchedManifest,
// and stores it iff it is newer than the baseline (the external file if
// configured, else the embedded catalog). The source tag records how the
// manifest was obtained ("startup" or "api") for diagnostics.
func fetchAndStore(url, source string) (bool, error) {
	cfg := config.GetServerConfig()

	client := &http.Client{Timeout: fetchTimeout}
	if cfg != nil && cfg.TLS.SkipVerify {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return false, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/toml, text/plain, */*")
	req.Header.Set("User-Agent", "knot/base-image-updater")

	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("fetching remote manifest (%s): %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("remote manifest returned HTTP %d (%s)", resp.StatusCode, url)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20)) // 4 MiB cap; manifest is tiny
	if err != nil {
		return false, fmt.Errorf("reading remote manifest: %w", err)
	}

	stored, err := SetFetchedManifest(body, source)
	if err != nil {
		return false, err
	}

	logger := log.WithGroup("base-images")
	if stored {
		logger.Info("manifest updated", "source", source, "url", url, "version", ActiveManifestVersion())
	} else {
		logger.Debug("fetched manifest not newer than baseline; ignored", "source", source, "url", url)
	}
	return stored, nil
}
