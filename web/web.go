package web

import (
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/origin_leaf/origin"
	"github.com/paularlott/knot/middleware"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

var (
	//go:embed public_html/api-docs public_html/assets public_html/images public_html/index.html
	publicHTML embed.FS

	//go:embed templates/*.tmpl templates/partials/*.tmpl templates/layouts/*.tmpl
	tmplFiles embed.FS

	//go:embed agents/*.zip
	agentFiles embed.FS
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

		// Serve index.html if path is empty
		fileName := strings.TrimPrefix(r.URL.Path, "/")
		if strings.HasSuffix(fileName, "/") || fileName == "" {
			fileName = fileName + "index.html"
		}

		// Add headers to allow caching for 4 hours
		w.Header().Set("Cache-Control", "public, max-age=14400")

		// If server.html_path is given then serve the files from that path otherwise serve the embedded files
		htmlPath := viper.GetString("server.html_path")
		if htmlPath != "" {
			// If the file does exist then return a 404
			info, err := os.Stat(filepath.Join(htmlPath, fileName))
			if os.IsNotExist(err) || info.IsDir() {
				showPageNotFound(w, r)
				return
			}

			// Calculate the ETag and set it
			etag := fmt.Sprintf("%x", info.ModTime().Unix())
			w.Header().Set("ETag", etag)

			// Check if the ETag matches and return 304 if it does
			if match := r.Header.Get("If-None-Match"); match == etag {
				w.WriteHeader(http.StatusNotModified)
				return
			}

			// Serve the file
			fs := http.StripPrefix(pathPrefix, http.FileServer(http.Dir(htmlPath)))
			fs.ServeHTTP(w, r)
		} else {
			fsys := fs.FS(publicHTML)
			contentStatic, _ := fs.Sub(fsys, "public_html")

			// Check if the file exists in the embedded files
			_, err := fs.Stat(contentStatic, fileName)
			if err != nil {
				showPageNotFound(w, r)
				return
			}

			// Set ETag header to the version
			w.Header().Set("ETag", build.Version)

			// Check if the ETag matches and return 304 if it does
			if match := r.Header.Get("If-None-Match"); match == build.Version {
				w.WriteHeader(http.StatusNotModified)
				return
			}

			fs := http.StripPrefix(pathPrefix, http.FileServer(http.FS(contentStatic)))
			fs.ServeHTTP(w, r)
		}
	})

	// Serve agent files
	router.Get("/agents/*", func(w http.ResponseWriter, r *http.Request) {
		rctx := chi.RouteContext(r.Context())
		pathPrefix := strings.TrimSuffix(rctx.RoutePattern(), "/*")
		fileName := strings.TrimPrefix(r.URL.Path, "/")

		agentPath := viper.GetString("server.agent_path")
		if agentPath != "" {
			// Strip agents/ from the path
			fileName = strings.TrimPrefix(fileName, "agents/")

			// If the file does exist then return a 404
			info, err := os.Stat(filepath.Join(agentPath, fileName))
			if os.IsNotExist(err) || info.IsDir() {
				showPageNotFound(w, r)
				return
			}

			// Serve the file
			fs := http.StripPrefix(pathPrefix, http.FileServer(http.Dir(agentPath)))
			fs.ServeHTTP(w, r)
		} else {
			// Check if the file exists in the embedded files
			fsys := fs.FS(agentFiles)
			contentStatic, _ := fs.Sub(fsys, "agents")
			_, err := fs.Stat(agentFiles, fileName)
			if err != nil {
				showPageNotFound(w, r)
				return
			}

			// Serve the file
			fs := http.StripPrefix(pathPrefix, http.FileServer(http.FS(contentStatic)))
			fs.ServeHTTP(w, r)
		}
	})

	// Group routes that require authentication
	router.Group(func(router chi.Router) {
		router.Use(middleware.WebAuth)

		router.Get("/clients", HandleSimplePage)
		router.Get("/sessions", HandleSimplePage)
		router.Get("/space-quota-reached", HandleSimplePage)
		router.Get("/profile", HandleUserProfilePage)
		router.Get("/logout", HandleLogoutPage)

		router.Get("/terminal/{space_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}", HandleTerminalPage)
		router.Get("/terminal/{space_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}/{vsc:vscode-tunnel}", HandleTerminalPage)

		router.Route("/api-tokens", func(router chi.Router) {
			router.Get("/", HandleSimplePage)
			router.Get("/create", HandleSimplePage)
			router.Get("/create/{token_name}", HandleTokenCreatePage)
		})

		router.Route("/spaces", func(router chi.Router) {
			router.Get("/", HandleListSpaces)
			router.Get("/{user_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}", HandleListSpaces)
			router.Get("/create/{template_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}", HandleSpacesCreate)
			router.Get("/create/{template_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}/{user_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}", HandleSpacesCreate)
			router.Get("/edit/{space_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}", HandleSpacesEdit)
		})

		router.Route("/templates", func(router chi.Router) {
			router.Use(checkPermissionManageTemplates)

			router.Get("/create", HandleTemplateCreate)
			router.Get("/edit/{template_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}", HandleTemplateEdit)
		})
		router.Get("/templates", HandleSimplePage)

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
			router.Get("/edit/{user_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}", HandleUserEdit)
		})

		router.Route("/groups", func(router chi.Router) {
			router.Use(checkPermissionManageUsers)

			router.Get("/", HandleSimplePage)
			router.Get("/create", HandleGroupCreate)
			router.Get("/edit/{group_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}", HandleGroupEdit)
		})

		router.Route("/volumes", func(router chi.Router) {
			router.Use(checkPermissionManageVolumes)

			router.Get("/", HandleSimplePage)
			router.Get("/create", HandleVolumeCreate)
			router.Get("/edit/{volume_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}", HandleVolumeEdit)
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
	err = tmpl.Execute(w, map[string]interface{}{
		"version": build.Version,
	})
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
	err = tmpl.Execute(w, map[string]interface{}{
		"version": build.Version,
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

// Initialize a new template
func newTemplate(name string) (*template.Template, error) {

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

	// If server.template_path is given then serve the files from that path otherwise serve the embedded files
	templatePath := viper.GetString("server.template_path")
	if templatePath != "" {
		// Check if template exists in the given template_path
		filePath := filepath.Join(templatePath, name)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			return nil, errors.New("template not found")
		}

		// Create the template from the given template_path
		tmpl, err := template.New(name).Funcs(funcs).ParseGlob(filepath.Join(templatePath, "layouts", "*.tmpl"))
		if err != nil {
			return nil, err
		}
		tmpl, err = tmpl.ParseGlob(filepath.Join(templatePath, "partials", "*.tmpl"))
		if err != nil {
			return nil, err
		}
		tmpl, err = tmpl.ParseFiles(filePath)
		if err != nil {
			return nil, err
		}
		return tmpl, nil
	}

	// Check if template exists
	file, err := tmplFiles.Open(fmt.Sprintf("templates/%s", name))
	if err != nil {
		return nil, errors.New("template not found")
	}
	file.Close()

	// Create the template
	tmpl, err := template.New(name).Funcs(funcs).ParseFS(tmplFiles, "templates/partials/*.tmpl", "templates/layouts/*.tmpl", fmt.Sprintf("templates/%s", name))
	if err != nil {
		return nil, err
	}

	return tmpl, err
}

func getCommonTemplateData(r *http.Request) (*model.User, map[string]interface{}) {
	user := r.Context().Value("user").(*model.User)

	withDownloads := false
	downloadPath := viper.GetString("server.download_path")
	if downloadPath != "" {
		withDownloads = true
	}

	return user, map[string]interface{}{
		"username":                  user.Username,
		"user_id":                   user.Id,
		"withDownloads":             withDownloads,
		"permissionManageUsers":     user.HasPermission(model.PermissionManageUsers) && !origin.RestrictedLeaf,
		"permissionManageTemplates": user.HasPermission(model.PermissionManageTemplates) && !origin.RestrictedLeaf,
		"permissionManageSpaces":    user.HasPermission(model.PermissionManageSpaces) && !origin.RestrictedLeaf,
		"permissionManageVolumes":   user.HasPermission(model.PermissionManageVolumes) || origin.RestrictedLeaf,
		"version":                   build.Version,
		"buildDate":                 build.Date,
		"hasRemoteToken":            viper.GetString("server.shared_token") != "",
		"location":                  viper.GetString("server.location"),
		"isOrigin":                  origin.IsOrigin,
		"isLeaf":                    origin.IsLeaf,
		"isOriginOrLeaf":            origin.IsOrigin || origin.IsLeaf,
		"isRestrictedServer":        origin.RestrictedLeaf,
	}
}
