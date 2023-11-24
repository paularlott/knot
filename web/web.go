package web

import (
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strings"

	"github.com/paularlott/knot/middleware"

	"github.com/go-chi/chi/v5"
)

var (
  //go:embed public_html/*
  publicHTML embed.FS

  //go:embed templates/*.tmpl
  tmplFiles embed.FS
)

func Routes() chi.Router {
  router := chi.NewRouter()

  // Page not found
  router.NotFound(showPageNotFound)

  // Serve static content
  router.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		rctx := chi.RouteContext(r.Context())
		pathPrefix := strings.TrimSuffix(rctx.RoutePattern(), "/*")

    fsys := fs.FS(publicHTML)
    contentStatic, _ := fs.Sub(fsys, "public_html")

    // Test if file r.URL.Path exists in contentStatic
    fileName := strings.TrimPrefix(r.URL.Path, "/")
    if fileName == "" {
      fileName = "index.html"
    }

    file, err := contentStatic.Open(fileName)
    if err != nil {
      showPageNotFound(w, r)
      return
    }
    file.Close()

		fs := http.StripPrefix(pathPrefix, http.FileServer(http.FS(contentStatic)))
		fs.ServeHTTP(w, r)
	})

  // Group routes that require authentication
  router.Group(func(router chi.Router) {
    router.Use(middleware.WebAuth)

    router.Get("/dashboard", HandleSimplePage)
    router.Get("/sessions", HandleSimplePage)
    router.Get("/logout", HandleLogoutPage)

    router.Route("/api-tokens", func(router chi.Router) {
      router.Get("/", HandleSimplePage)
      router.Get("/create", HandleSimplePage)
      router.Get("/create/{token_name}", HandleTokenCreatePage)
    })

    router.Route("/spaces", func(router chi.Router) {
      router.Get("/", HandleSimplePage)
      router.Get("/create", HandleSimplePage)
//      router.Get("/edit/{agent_id}", HandleAgentEditPage)
      router.HandleFunc("/{space_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}/code-server/*", HandleSpacesCodeServerProxy)
    })
  })

  // Routes without authentication
  router.Get("/initial-system-setup", HandleInitialSystemSetupPage)
  router.Get("/login", HandleLoginPage)
  router.Get("/health", HandleHealthPage)

  return router
}

func showPageNotFound(w http.ResponseWriter, r *http.Request) {
  tmpl, err := newTemplate("page-404.tmpl")
  if err != nil {
    w.WriteHeader(http.StatusInternalServerError)
    return
  }

  w.WriteHeader(http.StatusNotFound)
  err = tmpl.Execute(w, nil)
  if err != nil {
    w.WriteHeader(http.StatusInternalServerError)
    return
  }
}

// Initialize a new template
func newTemplate(name string) (*template.Template, error){

  // Add a function to allow passing of KV pairs to templates
  funcs := map[string]any{
		"map": func(pairs ...any) (map[string]any, error) {
			if len(pairs)%2 != 0 {
				return nil, errors.New("map requires key value pairs")
			}

			m := make(map[string]any, len(pairs)/2)

			for i := 0; i < len(pairs); i += 2 {
				key, ok := pairs[i].(string)

				if !ok {
					return nil, fmt.Errorf("type %T is not usable as map key", pairs[i])
				}
				m[key] = pairs[i+1]
			}
			return m, nil
		},
	}

  // Check if template exists
  file, err := tmplFiles.Open(fmt.Sprintf("templates/%s", name))
  if err != nil {
    return nil, nil
  }
  file.Close()

  // Create the template
  tmpl, err := template.New(name).Funcs(funcs).ParseFS(tmplFiles, "templates/*.tmpl")
  if err != nil {
    return nil, err
  }

  return tmpl, err
}
