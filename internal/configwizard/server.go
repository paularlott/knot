package configwizard

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	cli_toml "github.com/paularlott/cli/toml"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/log"
)

//go:embed static
var staticFS embed.FS

const shutdownTimeout = 5 * time.Second

// Options tunes wizard behaviour. The zero value matches the historical
// `knot server config-wizard` behaviour.
type Options struct {
	// TargetPath forces the output path instead of resolving via the
	// config search paths. Used by desktop mode (~/.knot/knot.toml).
	TargetPath string
	// AllowOverwrite permits saving when the target file already exists.
	// Used by desktop mode, which pre-fills the wizard from the existing
	// (incomplete) config and completes it.
	AllowOverwrite bool
	// Desktop selects the local-machine preset and desktop success
	// messaging (restart the app rather than run knot server).
	Desktop bool
	// BasePath is the URL prefix the wizard is served under ("/" for the
	// standalone server, "/setup/" when embedded in a running server).
	BasePath string
}

// buildForm composes the wizard form: desktop preset when requested,
// pre-filled from any existing config so an incomplete setup can be
// completed or a running setup re-edited.
func buildForm(o Options, configPath string, configExists bool) Form {
	form := DefaultForm()
	if o.Desktop {
		form = DesktopForm()
	}
	if configExists {
		form = FormFromConfig(form, configPath)
	}
	return form
}

// PageHandler returns a handler that renders the wizard page, building the
// form from the config file at request time (so re-runs reflect the current
// file). Intended for embedding in a running server behind its own auth.
func PageHandler(o Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		configPath, configExists := resolveTarget(o)
		renderWizard(w, buildForm(o, configPath, configExists), configPath, configExists, o)
	}
}

// SaveHandler returns a handler that validates and writes the submitted
// config to the target path. Intended for embedding in a running server
// behind its own auth; it does not stop anything.
func SaveHandler(o Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		configPath, configExists := resolveTarget(o)
		saveConfig(w, r, configPath, configExists, o)
	}
}

// StaticHandler returns a handler serving the wizard's embedded static
// assets; the mount prefix is stripped from the request path.
func StaticHandler() http.Handler {
	return http.StripPrefix("/setup/static", staticHandler())
}

// PreviewHandler returns a handler that merges the submitted TOML with the
// existing config without writing anything, so the wizard's review step can
// show the true final file — including sections the wizard doesn't manage.
func PreviewHandler(o Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		configPath, configExists := resolveTarget(o)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		content := r.PostFormValue("toml")
		if strings.TrimSpace(content) == "" {
			http.Error(w, "editor content is empty", http.StatusBadRequest)
			return
		}
		if configExists {
			if existing, err := os.ReadFile(configPath); err == nil {
				content = mergeConfig(string(existing), content)
			}
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprint(w, content)
	}
}

// resolveTarget determines the config path for the given options,
// mirroring Serve's resolution.
func resolveTarget(o Options) (string, bool) {
	if o.TargetPath != "" {
		_, err := os.Stat(o.TargetPath)
		return o.TargetPath, err == nil
	}
	return resolveConfig("")
}

func Serve(ctx context.Context, addr, configFlag string, opts ...Options) error {
	var o Options
	if len(opts) > 0 {
		o = opts[0]
	}
	if o.BasePath == "" {
		o.BasePath = "/"
	}

	var configPath string
	var configExists bool
	if o.TargetPath != "" {
		configPath = o.TargetPath
		_, err := os.Stat(configPath)
		configExists = err == nil
	} else {
		configPath, configExists = resolveConfig(configFlag)
	}

	form := buildForm(o, configPath, configExists)

	stop := make(chan struct{})

	mux := http.NewServeMux()
	mux.Handle("GET /static/", staticHandler())
	mux.HandleFunc("GET /", indexHandler(form, configPath, configExists, o))
	mux.HandleFunc("POST /preview", PreviewHandler(o))
	mux.HandleFunc("POST /save", func(w http.ResponseWriter, r *http.Request) {
		saveConfig(w, r, configPath, configExists, o)
		go closeOnce(stop)
	})
	mux.HandleFunc("GET /shutdown", shutdownHandler(stop))

	server := &http.Server{Addr: addr, Handler: logging(mux)}

	url := fmt.Sprintf("http://%s/", addr)
	fmt.Fprintf(os.Stderr, "\n  knot config wizard\n  Open this URL in your browser:\n    %s\n  (Ctrl+C to cancel)\n\n", url)
	log.Info("config wizard listening", "addr", addr, "config_target", configPath, "config_exists", configExists, "desktop", o.Desktop)

	errCh := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case <-stop:
		if o.Desktop {
			fmt.Fprintf(os.Stderr, "\n  Configuration written to %s\n  Quit knot from the tray menu and reopen it to apply.\n\n", configPath)
		} else {
			fmt.Fprintf(os.Stderr, "\n  Configuration written to %s\n  You can now start the server: knot server\n\n", configPath)
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

func resolveConfig(configFlag string) (string, bool) {
	name := configFlag
	if name == "" {
		name = config.CONFIG_FILE
	}
	src := cli_toml.NewConfigFile(&name, configSearchPaths)
	if err := src.LoadData(); err == nil {
		return src.FileUsed(), true
	}
	return name, false
}

func configSearchPaths() []string {
	paths := []string{"."}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		paths = append(paths, home)
		paths = append(paths, filepath.Join(home, "."+config.CONFIG_DIR))
		paths = append(paths, filepath.Join(home, ".config", config.CONFIG_DIR))
	}
	paths = append(paths, filepath.Join("/etc", config.CONFIG_DIR))
	return paths
}

func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Debug("wizard request", "method", r.Method, "path", r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

func staticHandler() http.Handler {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "static assets unavailable", http.StatusInternalServerError)
		})
	}
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/static/")
		fileServer.ServeHTTP(w, r)
	})
}

func indexHandler(form Form, configPath string, configExists bool, o Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// The wizard shares its port with the server's advertised address,
		// so browsers with stale URLs from a previous server session (e.g.
		// /login) land here — send them to the wizard instead of a 404.
		if r.URL.Path != "/" {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		renderWizard(w, form, configPath, configExists, o)
	}
}

// saveConfig validates and writes the submitted TOML, rendering either the
// success page or an error response. Shared by the standalone wizard and
// the in-server /setup mount.
func saveConfig(w http.ResponseWriter, r *http.Request, configPath string, configExists bool, o Options) {
	if configExists && !o.AllowOverwrite {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusConflict)
		fmt.Fprintf(w, "A configuration file already exists at %s — the wizard will not overwrite it.", configPath)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	content := r.PostFormValue("toml")
	if strings.TrimSpace(content) == "" {
		http.Error(w, "editor content is empty", http.StatusBadRequest)
		return
	}

	// When overwriting an existing config, merge rather than replace: the
	// wizard's values win for the keys it manages, but comments, unknown
	// keys and unknown sections the user added by hand are preserved.
	// merged=1 marks content that already IS the merged result (the
	// review editor's preview) — writing it verbatim honours deletions
	// the user made in the editor instead of resurrecting them from disk.
	if configExists && o.AllowOverwrite && r.PostFormValue("merged") != "1" {
		if existing, err := os.ReadFile(configPath); err == nil {
			content = mergeConfig(string(existing), content)
		} else {
			log.Error("reading existing config for merge, replacing it", "err", err, "path", configPath)
		}
	}

	var probe map[string]interface{}
	if err := toml.Unmarshal([]byte(content), &probe); err != nil {
		http.Error(w, "invalid TOML: "+err.Error(), http.StatusBadRequest)
		return
	}

	if problems := validateTomlConfig(probe); len(problems) > 0 {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Configuration is incomplete:\n  - %s", strings.Join(problems, "\n  - "))
		return
	}

	if err := writeConfig(configPath, content); err != nil {
		log.Error("writing config", "err", err)
		http.Error(w, "failed to write config file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	log.Info("config wizard wrote config", "path", configPath)
	renderSuccess(w, configPath, o)
}

func shutdownHandler(stop chan struct{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "shutting down")
		go closeOnce(stop)
	}
}

func closeOnce(ch chan struct{}) {
	select {
	case <-ch:
	default:
		close(ch)
	}
}

func writeConfig(path, content string) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
