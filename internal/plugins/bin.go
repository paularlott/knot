package plugins

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/paularlott/scriptling/plugin"
)

// Peer binaries under bin/ ship in one of three package shapes: a single bare binary; a platform bundle with _<goarch> variants; or
// a universal bundle with _<goos>_<goarch> variants. A package never mixes
// bare and variants of the same peer — bare says "this is the one binary",
// variants say "pick mine". Resolution: bare wins when present, else the
// fully specific variant, else the arch-only variant.

var knownGoarch = map[string]bool{
	"amd64": true, "arm64": true, "arm": true, "386": true,
	"loong64": true, "ppc64le": true, "ppc64": true, "riscv64": true, "s390x": true,
}

var knownGoos = map[string]bool{
	"linux": true, "darwin": true, "windows": true, "freebsd": true,
	"openbsd": true, "netbsd": true, "dragonfly": true, "solaris": true, "aix": true,
}

// resolvedPeer is one executable this host should spawn.
type resolvedPeer struct {
	name string // discovery base name (handshake name may differ)
	path string
}

// variantKind classifies a bin/ entry for its group.
type variantKind int

const (
	variantBare variantKind = iota
	variantGoarch
	variantGoosGoarch
)

// stripVariant removes the exe extension and any arch/os suffix, returning
// the base name and what was stripped.
func stripVariant(file string) (base string, kind variantKind, goos, goarch string) {
	name := strings.TrimSuffix(file, ".exe")
	// Longest first so _linux_arm64 is never read as base "_linux" + arch.
	if i := strings.LastIndex(name, "_"); i > 0 {
		last := name[i+1:]
		if knownGoarch[last] {
			rest := name[:i]
			if j := strings.LastIndex(rest, "_"); j > 0 {
				maybeGoos := rest[j+1:]
				if knownGoos[maybeGoos] && knownGoarch[last] {
					return rest[:j], variantGoosGoarch, maybeGoos, last
				}
			}
			return rest, variantGoarch, "", last
		}
	}
	return name, variantBare, "", ""
}

// resolveBinPeers walks bin/ and picks one executable per peer group for
// this host. Non-executable files are companions (implementation modules,
// data files) and are skipped silently; variants for other hosts are
// skipped silently too.
func resolveBinPeers(binDir string) ([]resolvedPeer, []string) {
	entries, err := os.ReadDir(binDir)
	if err != nil {
		return nil, nil // no bin/ folder: no peers
	}

	var warnings []string
	type group struct {
		bare       string
		goarch     map[string]string
		goosGoarch map[string]string
	}
	groups := map[string]*group{}

	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		path := filepath.Join(binDir, entry.Name())
		if info, err := entry.Info(); err == nil && info.Mode()&0o111 == 0 {
			// Only executables are peers; a non-executable file in bin/ is a
			// companion, not a broken peer — a scriptling peer's .py
			// implementation module, the sqlite/data file a peer persists
			// next to itself, a README, whatever. Skipped silently: knot
			// never tries to spawn it, so there is nothing to warn about.
			continue
		}
		base, kind, goos, goarch := stripVariant(entry.Name())
		g, ok := groups[base]
		if !ok {
			g = &group{goarch: map[string]string{}, goosGoarch: map[string]string{}}
			groups[base] = g
		}
		switch kind {
		case variantBare:
			g.bare = path
		case variantGoarch:
			g.goarch[goarch] = path
		case variantGoosGoarch:
			g.goosGoarch[goos+"_"+goarch] = path
		}
	}

	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)

	var peers []resolvedPeer
	for _, name := range names {
		g := groups[name]
		hasVariants := len(g.goarch) > 0 || len(g.goosGoarch) > 0
		switch {
		case g.bare != "":
			if hasVariants {
				warnings = append(warnings, fmt.Sprintf("bin: peer %q ships both a bare binary and variants — malformed package, bare wins", name))
			}
			peers = append(peers, resolvedPeer{name: name, path: g.bare})
		case g.goosGoarch[runtime.GOOS+"_"+runtime.GOARCH] != "":
			path := g.goosGoarch[runtime.GOOS+"_"+runtime.GOARCH]
			peers = append(peers, resolvedPeer{name: name, path: path})
		case g.goarch[runtime.GOARCH] != "":
			peers = append(peers, resolvedPeer{name: name, path: g.goarch[runtime.GOARCH]})
		default:
			// Only variants for other hosts: absent on this host.
		}
	}
	return peers, warnings
}

// loadPeers spawns and handshakes a plugin's bin/ binaries into a scope
// owned by the plugin (invisible to other plugins). A binary that fails to
// spawn or handshake is a warning here; if the plugin's metadata declared a
// requirement on it, verification afterwards fails the plugin through the
// normal path.
func loadPeers(pluginName, pluginDir string, newManager func() *plugin.Manager) (*plugin.Manager, []string, error) {
	binDir := filepath.Join(pluginDir, "bin")
	peers, warnings := resolveBinPeers(binDir)
	if len(peers) == 0 {
		return nil, warnings, nil
	}

	scope := newManager().NewScope(plugin.WithTransport(plugin.TransportStdio))
	for _, peer := range peers {
		ctx, cancel := context.WithTimeout(context.Background(), peerLoadTimeout)
		if _, err := scope.LoadPlugin(ctx, peer.path, nil); err != nil {
			warnings = append(warnings, fmt.Sprintf("bin/%s failed to load: %v", filepath.Base(peer.path), err))
		}
		cancel()
	}
	for _, w := range scope.Warnings() {
		warnings = append(warnings, "bin: "+w)
	}
	return scope, warnings, nil
}
