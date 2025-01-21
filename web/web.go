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
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/origin_leaf/server_info"
	"github.com/paularlott/knot/middleware"

	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

var (
	//go:embed public_html/api-docs public_html/assets public_html/images public_html/index.html
	//go:embed public_html/site.webmanifest public_html/favicon.ico public_html/*.png
	publicHTML embed.FS

	//go:embed templates/*.tmpl templates/partials/*.tmpl templates/layouts/*.tmpl
	tmplFiles embed.FS

	//go:embed agents/*.zip
	agentFiles embed.FS
)

func HandlePageNotFound(next *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, pattern := next.Handler(r)
		if pattern == "" {
			showPageNotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func Routes(router *http.ServeMux) {
	log.Info().Msg("server: adding routes")

	router.HandleFunc("GET /health", HandleHealthPage)

	// Serve static content
	router.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		// Serve index.html if path is empty
		fileName := strings.TrimPrefix(r.URL.Path, "/")
		if strings.HasSuffix(fileName, "/") || fileName == "" {
			fileName = fileName + "index.html"
		}

		if strings.Contains(fileName, "..") {
			http.Error(w, "Invalid file name", http.StatusBadRequest)
			return
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
			fs := http.FileServer(http.Dir(htmlPath))
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

			fs := http.FileServer(http.FS(contentStatic))
			fs.ServeHTTP(w, r)
		}
	})

	// Serve agent files
	router.HandleFunc("GET /agents/{agent_file}", func(w http.ResponseWriter, r *http.Request) {
		fileName := r.PathValue("agent_file")

		if strings.Contains(fileName, "..") {
			http.Error(w, "Invalid file name", http.StatusBadRequest)
			return
		}

		agentPath := viper.GetString("server.agent_path")
		agentPath = ""
		if agentPath != "" {
			// If the file does exist then return a 404
			info, err := os.Stat(filepath.Join(agentPath, fileName))
			if os.IsNotExist(err) || info.IsDir() {
				showPageNotFound(w, r)
				return
			}

			// Serve the file
			fs := http.StripPrefix("/agents", http.FileServer(http.Dir(agentPath)))
			fs.ServeHTTP(w, r)
		} else {
			// Check if the file exists in the embedded files
			fsys := fs.FS(agentFiles)
			contentStatic, _ := fs.Sub(fsys, "agents")
			_, err := fs.Stat(agentFiles, "agents/"+fileName)
			if err != nil {
				showPageNotFound(w, r)
				return
			}

			// Serve the file
			fs := http.StripPrefix("/agents", http.FileServer(http.FS(contentStatic)))
			fs.ServeHTTP(w, r)
		}
	})

	// Group routes that require authentication
	router.HandleFunc("GET /clients", middleware.WebAuth(HandleSimplePage))
	router.HandleFunc("GET /sessions", middleware.WebAuth(HandleSimplePage))
	router.HandleFunc("GET /space-quota-reached", middleware.WebAuth(HandleSimplePage))
	router.HandleFunc("GET /profile", middleware.WebAuth(HandleUserProfilePage))
	router.HandleFunc("GET /logout", middleware.WebAuth(HandleLogoutPage))
	router.HandleFunc("GET /usage", middleware.WebAuth(HandleSimplePage))

	router.HandleFunc("GET /terminal/{space_id}", middleware.WebAuth(HandleTerminalPage))
	router.HandleFunc("GET /terminal/{space_id}/{vsc}", middleware.WebAuth(HandleTerminalPage))

	router.HandleFunc("GET /api-tokens", middleware.WebAuth(HandleSimplePage))
	router.HandleFunc("GET /api-tokens/create", middleware.WebAuth(HandleSimplePage))
	router.HandleFunc("GET /api-tokens/create/{token_name}", middleware.WebAuth(HandleTokenCreatePage))

	router.HandleFunc("GET /spaces", middleware.WebAuth(HandleListSpaces))
	router.HandleFunc("GET /spaces/{user_id}", middleware.WebAuth(checkPermissionUseManageSpaces(HandleListSpaces)))
	router.HandleFunc("GET /spaces/create/{template_id}", middleware.WebAuth(checkPermissionUseManageSpaces(HandleSpacesCreate)))
	router.HandleFunc("GET /spaces/create/{template_id}/{user_id}", middleware.WebAuth(checkPermissionUseManageSpaces(HandleSpacesCreate)))
	router.HandleFunc("GET /spaces/edit/{space_id}", middleware.WebAuth(checkPermissionUseManageSpaces(HandleSpacesEdit)))

	router.HandleFunc("GET /templates", middleware.WebAuth(HandleSimplePage))
	router.HandleFunc("GET /templates/create", middleware.WebAuth(checkPermissionManageTemplates(HandleTemplateCreate)))
	router.HandleFunc("GET /templates/edit/{template_id}", middleware.WebAuth(checkPermissionManageTemplates(HandleTemplateEdit)))

	router.HandleFunc("GET /variables", middleware.WebAuth(checkPermissionManageVariables(HandleSimplePage)))
	router.HandleFunc("GET /variables/create", middleware.WebAuth(checkPermissionManageVariables(HandleTemplateVarCreate)))
	router.HandleFunc("GET /variables/edit/{templatevar_id}", middleware.WebAuth(checkPermissionManageVariables(HandleTemplateVarEdit)))

	router.HandleFunc("GET /users", middleware.WebAuth(checkPermissionManageUsers(HandleSimplePage)))
	router.HandleFunc("GET /users/create", middleware.WebAuth(checkPermissionManageUsers(HandleUserCreate)))
	router.HandleFunc("GET /users/edit/{user_id}", middleware.WebAuth(checkPermissionManageUsers(HandleUserEdit)))

	router.HandleFunc("GET /groups", middleware.WebAuth(checkPermissionManageGroups(HandleSimplePage)))
	router.HandleFunc("GET /groups/create", middleware.WebAuth(checkPermissionManageGroups(HandleGroupCreate)))
	router.HandleFunc("GET /groups/edit/{group_id}", middleware.WebAuth(checkPermissionManageGroups(HandleGroupEdit)))

	router.HandleFunc("GET /roles", middleware.WebAuth(checkPermissionManageRoles(HandleSimplePage)))
	router.HandleFunc("GET /roles/create", middleware.WebAuth(checkPermissionManageRoles(HandleRoleCreate)))
	router.HandleFunc("GET /roles/edit/{role_id}", middleware.WebAuth(checkPermissionManageRoles(HandleRoleEdit)))

	router.HandleFunc("GET /volumes", middleware.WebAuth(checkPermissionManageVolumes(HandleSimplePage)))
	router.HandleFunc("GET /volumes/create", middleware.WebAuth(checkPermissionManageVolumes(HandleVolumeCreate)))
	router.HandleFunc("GET /volumes/edit/{volume_id}", middleware.WebAuth(checkPermissionManageVolumes(HandleVolumeEdit)))

	router.HandleFunc("GET /logs/{space_id}", middleware.WebAuth(HandleLogsPage))

	if viper.GetString("server.listen_tunnel") != "" {
		router.HandleFunc("GET /tunnels", middleware.WebAuth(checkPermissionUseTunnels(HandleSimplePage)))
	}

	if database.GetInstance().HasAuditLog() {
		router.HandleFunc("GET /audit-logs", middleware.WebAuth(checkPermissionViewAuditLogs(HandleSimplePage)))
	}

	router.HandleFunc("GET /logs/{space_id}/stream", middleware.ApiAuth(HandleLogsStream))

	// Routes without authentication
	if !middleware.HasUsers {
		router.HandleFunc("GET /initial-system-setup", HandleInitialSystemSetupPage)
	}
	router.HandleFunc("GET /login", HandleLoginPage)

	// If download path set then enable serving of the download folder
	downloadPath := viper.GetString("server.download_path")
	if downloadPath != "" {
		log.Info().Msgf("server: enabling download endpoint, source folder %s", downloadPath)

		router.HandleFunc("GET /download/", func(w http.ResponseWriter, r *http.Request) {
			filePath := r.URL.Path[len("/download/"):]

			if strings.Contains(filePath, "..") {
				http.Error(w, "Invalid file name", http.StatusBadRequest)
				return
			}

			http.ServeFile(w, r, filepath.Join(downloadPath, filePath))
		})
	}
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

	if strings.Contains(name, "..") {
		return nil, errors.New("invalid template name")
	}

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
		"permissionManageUsers":     user.HasPermission(model.PermissionManageUsers) && !server_info.RestrictedLeaf,
		"permissionManageGroups":    user.HasPermission(model.PermissionManageGroups) && !server_info.RestrictedLeaf,
		"permissionManageRoles":     user.HasPermission(model.PermissionManageRoles) && !server_info.RestrictedLeaf,
		"permissionManageTemplates": user.HasPermission(model.PermissionManageTemplates) && !server_info.RestrictedLeaf,
		"permissionManageVariables": user.HasPermission(model.PermissionManageVariables) && !server_info.RestrictedLeaf,
		"permissionManageSpaces":    user.HasPermission(model.PermissionManageSpaces) && !server_info.RestrictedLeaf,
		"permissionManageVolumes":   user.HasPermission(model.PermissionManageVolumes) || server_info.RestrictedLeaf,
		"permissionUseSpaces":       user.HasPermission(model.PermissionUseSpaces) || user.HasPermission(model.PermissionManageSpaces) || server_info.RestrictedLeaf,
		"permissionUseTunnels":      user.HasPermission(model.PermissionUseTunnels) && viper.GetString("server.listen_tunnel") != "",
		"permissionViewAuditLogs":   user.HasPermission(model.PermissionViewAuditLogs) && !server_info.RestrictedLeaf && database.GetInstance().HasAuditLog(),
		"version":                   build.Version,
		"buildDate":                 build.Date,
		"location":                  server_info.LeafLocation,
		"isOrigin":                  server_info.IsOrigin,
		"isLeaf":                    server_info.IsLeaf,
		"isOriginOrLeaf":            server_info.IsOrigin || server_info.IsLeaf,
		"isRestrictedServer":        server_info.RestrictedLeaf,
		"timezone":                  server_info.Timezone,
		"disableSpaceCreate":        viper.GetBool("server.disable_space_create"),
	}
}
