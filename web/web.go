package web

import (
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/middleware"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"

	"github.com/go-chi/chi/v5"
)

var (
  //go:embed public_html/*
  publicHTML embed.FS

  //go:embed templates/*.tmpl
  tmplFiles embed.FS
)

func Routes() chi.Router {
  log.Info().Msg("server: adding routes")

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
    if strings.HasSuffix(fileName, "/") || fileName == "" {
      fileName = fileName + "index.html"
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

    router.Get("/sessions", HandleSimplePage)
    router.Get("/logout", HandleLogoutPage)

    router.Get("/terminal/{space_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}", HandleTerminalPage)

    router.Route("/api-tokens", func(router chi.Router) {
      router.Get("/", HandleSimplePage)
      router.Get("/create", HandleSimplePage)
      router.Get("/create/{token_name}", HandleTokenCreatePage)
    })

    router.Route("/spaces", func(router chi.Router) {
      router.Get("/", HandleListSpaces)
      router.Get("/{user_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}", HandleListSpaces)
      router.Get("/create", HandleSpacesCreate)
      router.Get("/create/{user_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}", HandleSpacesCreate)
      router.Get("/edit/{space_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}", HandleSpacesEdit)
    })

    router.Route("/templates", func(router chi.Router) {
      router.Use(checkPermissionManageTemplates)

      router.Get("/", HandleSimplePage)
      router.Get("/create", HandleTemplateCreate)
      router.Get("/edit/{template_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}", HandleTemplateEdit)
    })

    router.Route("/variables", func(router chi.Router) {
      router.Use(checkPermissionManageTemplates)

      router.Get("/", HandleSimplePage)
      router.Get("/create", HandleTemplateVarCreate)
      router.Get("/edit/{templatevar_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}", HandleTemplateVarEdit)
    })

    router.Route("/users", func(router chi.Router) {
      router.Use(checkPermissionManageUsers)

      router.Get("/", HandleSimplePage)
      router.Get("/create", HandleUserCreate)
    })
    router.Route("/users/edit/{user_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}", func(router chi.Router) {
      router.Use(checkPermissionManageUsersOrSelf)

      router.Get("/", HandleUserEdit)
    })

    router.Route("/groups", func(router chi.Router) {
      router.Use(checkPermissionManageUsers)

      router.Get("/", HandleSimplePage)
      router.Get("/create", HandleGroupCreate)
      router.Get("/edit/{group_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}", HandleGroupEdit)
    })
  })

  // Routes without authentication
  if !middleware.HasUsers {
    router.Get("/initial-system-setup", HandleInitialSystemSetupPage)
  }
  router.Get("/login", HandleLoginPage)
  router.Get("/health", HandleHealthPage)

  // If download path set then enable serving of the download folder
  downloadPath := viper.GetString("server.download_path")
  if downloadPath != "" {
    log.Info().Msgf("server: enabling download endpoint, source folder %s", downloadPath)

    router.Get("/download/*", func(w http.ResponseWriter, r *http.Request) {
      filePath := r.URL.Path[len("/download/"):]
      http.ServeFile(w, r, filepath.Join(downloadPath, filePath))
    })
  }

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

func showPageForbidden(w http.ResponseWriter, r *http.Request) {
  tmpl, err := newTemplate("page-403.tmpl")
  if err != nil {
    w.WriteHeader(http.StatusInternalServerError)
    return
  }

  w.WriteHeader(http.StatusForbidden)
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

func getCommonTemplateData(r *http.Request) (*model.User, map[string]interface{}) {
  user := r.Context().Value("user").(*model.User)

  return user, map[string]interface{}{
    "username"                 : user.Username,
    "user_id"                  : user.Id,
    "permissionManageUsers"    : user.HasPermission(model.PermissionManageUsers),
    "permissionManageTemplates": user.HasPermission(model.PermissionManageTemplates),
    "permissionManageSpaces"   : user.HasPermission(model.PermissionManageSpaces),
  }
}
