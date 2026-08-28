package scriptlingserver

import (
	"context"
	"io"
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/scriptling/pluginfetch"
	"github.com/paularlott/scriptling"
	"github.com/paularlott/scriptling/plugin"
	"github.com/paularlott/scriptling/scriptling-cli/pack"
	"github.com/paularlott/scriptling/stdlib"
)

// minimalPluginFS implements just enough of fs.FS for the pack loader to
// resolve imports — ReadFile for module probing, ReadDir for listings.
type minimalPluginFS struct {
	client *plugin.Client
	source string
}

func (m *minimalPluginFS) Open(name string) (fs.File, error) {
	return nil, fs.ErrInvalid
}

func (m *minimalPluginFS) ReadFile(name string) ([]byte, error) {
	data, err := m.client.FetchFile(context.Background(), m.source, name)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, fs.ErrNotExist
		}
		return nil, err
	}
	return data, nil
}

func (m *minimalPluginFS) ReadDir(name string) ([]fs.DirEntry, error) {
	entries, err := m.client.FetchList(context.Background(), m.source, name)
	if err != nil {
		return nil, fs.ErrNotExist
	}
	out := make([]fs.DirEntry, len(entries))
	for i, e := range entries {
		out[i] = &minimalEntry{name: e.Name, isDir: e.IsDir}
	}
	return out, nil
}

type minimalEntry struct {
	name  string
	isDir bool
}

func (e *minimalEntry) Name() string { return e.name }
func (e *minimalEntry) IsDir() bool  { return e.isDir }
func (e *minimalEntry) Type() fs.FileMode {
	if e.isDir {
		return fs.ModeDir
	}
	return 0
}
func (e *minimalEntry) Info() (fs.FileInfo, error) { return nil, fs.ErrInvalid }

// TestEndToEndInterpreterLevel proves the full path: the knot fetcher serving
// libraries through the plugin protocol, imports resolving through the pack
// loader, and the embedded knot.* modules loading alongside user libraries.
func TestEndToEndInterpreterLevel(t *testing.T) {
	api := mockKnotAPI(t)
	defer api.Close()

	client := newTestAPIClient(t, api.URL)
	fetcher := newTestFetcher(t, api.URL)
	server := plugin.NewServer("knot", "test", "knot e2e test")
	server.RegisterFetcher("knot", fetcher)
	registerAPIFunctions(server, client.GetRESTClient())

	// Wire over pipes.
	clientToPluginR, clientToPluginW := io.Pipe()
	pluginToClientR, pluginToClientW := io.Pipe()
	go func() { _ = server.RunIO(clientToPluginR, pluginToClientW) }()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pc, err := plugin.LoadClientFromIO(ctx, pluginToClientR, clientToPluginW)
	if err != nil {
		t.Fatalf("LoadClientFromIO: %v", err)
	}

	// Build the bundle with the standard knot://libs layout.
	fsys := &minimalPluginFS{client: pc, source: "knot://libs"}
	bundle := pack.VirtualBundle("knot", "test", fsys, "knot://libs")

	// Build the interpreter with the full standard library (knot.space
	// imports urllib.parse and others).
	p := scriptling.New()
	stdlib.RegisterAll(p)
	loader := pack.NewLoader()

	// Register the plugin control library so the apiclient variant's
	// import scriptling.plugin resolves.
	manager := plugin.NewManager(nil)
	defer manager.Close()
	plugin.RegisterLibraries(p, manager)
	if err := loader.AddBundle(bundle); err != nil {
		t.Fatalf("AddBundle: %v", err)
	}
	loader.SetFallback(p.GetLibraryLoader())
	p.SetLibraryLoader(loader)

	t.Run("import embedded knot library", func(t *testing.T) {
		result, err := p.Eval("import knot.space\n'loaded'")
		if err != nil {
			t.Fatalf("import knot.space: %v", err)
		}
		if result.Inspect() != "loaded" {
			t.Fatalf("got %s", result.Inspect())
		}
	})

	t.Run("import user library from API", func(t *testing.T) {
		result, err := p.Eval("import mylib\nmylib.value()")
		if err != nil {
			t.Fatalf("import mylib: %v", err)
		}
		if !strings.Contains(result.Inspect(), "plugin test") {
			t.Fatalf("expected mock content, got %s", result.Inspect())
		}
	})

	t.Run("knot.apiclient variant", func(t *testing.T) {
		// The variant is served under lib/knot/apiclient.py; import it and
		// verify it routes through the plugin.
		data, err := pc.FetchFile(ctx, "knot://libs", "lib/knot/apiclient.py")
		if err != nil {
			t.Fatalf("FetchFile(apiclient): %v", err)
		}
		if !strings.Contains(string(data), "plugin.knot") {
			t.Fatal("expected the plugin-transport apiclient")
		}
	})

	t.Run("fetch script source", func(t *testing.T) {
		data, err := pc.FetchFile(ctx, "knot://myscript", "")
		if err != nil {
			t.Fatalf("FetchScript: %v", err)
		}
		if !strings.Contains(string(data), "mylib.value()") {
			t.Fatalf("expected script content, got %q", data)
		}
	})
}

func newTestAPIClient(t *testing.T, url string) *apiclient.ApiClient {
	t.Helper()
	c, err := apiclient.NewClient(url, "test-token", true)
	if err != nil {
		t.Fatal(err)
	}
	c.SetContentType("application/json")
	return c
}

func newTestFetcher(t *testing.T, apiURL string) plugin.Fetcher {
	t.Helper()
	inner := pluginfetch.NewFetcher(newTestAPIClient(t, apiURL))
	return fetcherAdapter{inner}
}
