package plugins

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	pluginpkg "github.com/paularlott/scriptling/plugin"
)

// TestAssetSource pins the declared-asset read order: a fetcher-serving
// peer wins (bytes cached for the HTTP handler), a fetch miss falls back to
// the plugin folder on disk, and without a peer the disk is the only
// source. The disk file remains the last word on existence.
func TestAssetSource(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "from-disk.svg"), []byte("<svg>disk</svg>"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// No peer: straight from disk.
	src := &assetSource{dir: dir}
	if b, err := src.Read(ctx, "from-disk.svg"); err != nil || string(b) != "<svg>disk</svg>" {
		t.Fatalf("disk read = %q, %v", b, err)
	}
	if len(src.cache) != 0 {
		t.Errorf("disk bytes must not be cached: %+v", src.cache)
	}
	if _, err := src.Read(ctx, "missing.svg"); err == nil {
		t.Error("missing asset should error")
	}

	// Peer serves some paths: a hit wins and is cached; a not-found falls
	// back to disk; anything else also degrades to disk.
	src = &assetSource{
		dir: dir,
		fetch: func(ctx context.Context, path string) ([]byte, error) {
			switch path {
			case "peer-only.svg":
				return []byte("<svg>peer</svg>"), nil
			default:
				return nil, pluginpkg.ErrFetchNotFound
			}
		},
	}
	if b, err := src.Read(ctx, "peer-only.svg"); err != nil || string(b) != "<svg>peer</svg>" {
		t.Fatalf("peer read = %q, %v", b, err)
	}
	if len(src.cache["peer-only.svg"]) == 0 {
		t.Error("peer-served bytes should be cached for the HTTP handler")
	}
	if b, err := src.Read(ctx, "from-disk.svg"); err != nil || string(b) != "<svg>disk</svg>" {
		t.Fatalf("fetch miss should fall back to disk, got %q, %v", b, err)
	}

	// A refusing/unavailable peer never invents an asset: disk decides.
	src = &assetSource{
		dir: dir,
		fetch: func(ctx context.Context, path string) ([]byte, error) {
			return nil, errors.New("backend on fire")
		},
	}
	if _, err := src.Read(ctx, "missing.svg"); err == nil {
		t.Error("a sick peer must not make a missing asset load")
	}
	if b, err := src.Read(ctx, "from-disk.svg"); err != nil || string(b) != "<svg>disk</svg>" {
		t.Fatalf("sick peer should degrade to disk, got %q, %v", b, err)
	}
}
