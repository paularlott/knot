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

func Serve(ctx context.Context, addr, configFlag string) error {
	configPath, configExists := resolveConfig(configFlag)
	stop := make(chan struct{})

	mux := http.NewServeMux()
	mux.Handle("GET /static/", staticHandler())
	mux.HandleFunc("GET /", indexHandler(configPath, configExists))
	mux.HandleFunc("POST /save", saveHandler(configPath, configExists, stop))
	mux.HandleFunc("GET /shutdown", shutdownHandler(stop))

	server := &http.Server{Addr: addr, Handler: logging(mux)}

	url := fmt.Sprintf("http://%s/", addr)
	fmt.Fprintf(os.Stderr, "\n  knot config wizard\n  Open this URL in your browser:\n    %s\n  (Ctrl+C to cancel)\n\n", url)
	log.Info("config wizard listening", "addr", addr, "config_target", configPath, "config_exists", configExists)

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
		fmt.Fprintf(os.Stderr, "\n  Configuration written to %s\n  You can now start the server: knot server\n\n", configPath)
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

func indexHandler(configPath string, configExists bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		renderWizard(w, DefaultForm(), configPath, configExists)
	}
}

func saveHandler(configPath string, configExists bool, stop chan struct{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if configExists {
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

		var probe map[string]interface{}
		if err := toml.Unmarshal([]byte(content), &probe); err != nil {
			http.Error(w, "invalid TOML: "+err.Error(), http.StatusBadRequest)
			return
		}

		if err := writeConfig(configPath, content); err != nil {
			log.Error("writing config", "err", err)
			http.Error(w, "failed to write config file: "+err.Error(), http.StatusInternalServerError)
			return
		}

		log.Info("config wizard wrote config", "path", configPath)
		renderSuccess(w, configPath)
		go closeOnce(stop)
	}
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
