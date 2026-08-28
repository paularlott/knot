// Package pluginfetch implements the knot plugin's fetcher: the source of
// truth for knot:// scheme sources served to the scriptling CLI. The embedded
// knot.* libraries come from the binary; user and global libraries and
// scripts come from the knot server API on demand.
package pluginfetch

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/scriptling"
)

// Fetcher serves knot:// sources for the scriptling plugin protocol.
//
// The layout is the standard one: modules under lib/, scripts as bare
// knot://name sources. Under lib/ sit two populations side by side:
//
//	lib/knot/<module>.py  — the embedded knot.* API libraries (the embedded
//	                        copy of apiclient.py is swapped for the
//	                        plugin-transport variant, so API calls route
//	                        through this plugin instead of direct HTTP)
//	lib/<name>.py          — the user's own and global library scripts,
//	                        fetched from the server on demand
type Fetcher struct {
	client *apiclient.ApiClient

	// libNames caches the server-side library enumeration (the list of
	// user + global lib script names) for the process lifetime, mirroring
	// the libs.zip merge: active, visible, user overriding global on name
	// collision.
	libOnce  sync.Once
	libNames []string
	libErr   error
}

// NewFetcher creates a fetcher backed by the given API client.
func NewFetcher(client *apiclient.ApiClient) *Fetcher {
	return &Fetcher{client: client}
}

// Read implements plugin.Fetcher. An empty path means the source itself is a
// single script file: knot://myscript fetches the script of that name.
func (f *Fetcher) Read(ctx context.Context, source, path string) ([]byte, error) {
	if path == "" {
		// Strip the scheme prefix: knot://myscript → myscript.
		name := strings.TrimPrefix(source, "knot://")
		if name == "" || strings.Contains(name, "/") {
			return nil, fmt.Errorf("%w: %s", errNotFound, source)
		}
		content, err := f.client.GetScriptByName(ctx, name)
		if err != nil {
			return nil, fmt.Errorf("%w: %s", errNotFound, name)
		}
		return []byte(content), nil
	}

	// lib/knot/<module>.py: the embedded libraries, with the apiclient
	// variant swapped in.
	if module, ok := strings.CutPrefix(path, "lib/knot/"); ok {
		if module == "apiclient.py" {
			return apiclientPluginSource, nil
		}
		data, err := scriptling.EmbeddedLibs.ReadFile("lib/knot/" + module)
		if err != nil {
			return nil, fmt.Errorf("%w: %s", errNotFound, path)
		}
		return data, nil
	}

	// lib/<name>.py: the user's or a global library script. The API serves
	// by script name, so the .py extension the loader adds is stripped.
	if name, ok := strings.CutPrefix(path, "lib/"); ok {
		if strings.Contains(name, "/") {
			return nil, fmt.Errorf("%w: %s", errNotFound, path)
		}
		name = strings.TrimSuffix(name, ".py")
		content, err := f.client.GetScriptLibrary(ctx, name)
		if err != nil {
			return nil, fmt.Errorf("%w: %s", errNotFound, name)
		}
		return []byte(content), nil
	}

	return nil, fmt.Errorf("%w: %s", errNotFound, path)
}

// List implements plugin.Fetcher. Listings enumerate the lib/ directory:
// the knot/ subdirectory plus the user's and global library names.
func (f *Fetcher) List(ctx context.Context, source, path string) ([]Entry, error) {
	switch path {
	case "", ".":
		return []Entry{{Name: "lib", IsDir: true}}, nil
	case "lib":
		names, err := f.libraryNames(ctx)
		if err != nil {
			return nil, err
		}
		entries := []Entry{{Name: "knot", IsDir: true}}
		for _, name := range names {
			entries = append(entries, Entry{Name: name + ".py"})
		}
		return entries, nil
	case "lib/knot":
		entries := make([]Entry, 0, 20)
		for _, name := range embeddedModuleNames() {
			if name != "apiclient" {
				entries = append(entries, Entry{Name: name + ".py"})
			}
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
		return entries, nil
	}
	return nil, fmt.Errorf("%w: %s in %s", errNotFound, path, source)
}

// Entry mirrors plugin.FetchEntry without importing the scriptling module
// here (the command wires them together).
type Entry struct {
	Name  string
	IsDir bool
}

// errNotFound is the sentinel the command wraps with the plugin package's
// ErrFetchNotFound. Kept local so this package has no scriptling dependency.
var errNotFound = fmt.Errorf("fetch source not found")

// libraryNames enumerates the user's + global library scripts from the
// server, once per process. The API list already scopes visibility, so the
// only merge rule is: user wins on name collision with a global.
func (f *Fetcher) libraryNames(ctx context.Context) ([]string, error) {
	f.libOnce.Do(func() {
		list, err := f.client.GetScripts(ctx)
		if err != nil {
			f.libErr = err
			return
		}
		seen := map[string]bool{}
		for _, script := range list.Scripts {
			if script.ScriptType != "lib" || !script.Active {
				continue
			}
			seen[script.Name] = true
		}
		f.libNames = make([]string, 0, len(seen))
		for name := range seen {
			f.libNames = append(f.libNames, name)
		}
		sort.Strings(f.libNames)
	})
	return f.libNames, f.libErr
}

// embeddedModuleNames returns the knot.* module names from the embedded FS.
func embeddedModuleNames() []string {
	entries, err := scriptling.EmbeddedLibs.ReadDir("lib/knot")
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".py")
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}
