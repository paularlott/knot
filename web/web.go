package web

import (
	"crypto/md5"
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
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/middleware"
	"github.com/paularlott/knot/internal/oauth2"

	"github.com/paularlott/knot/internal/log"
)

var (
	//go:embed public_html/api-docs public_html/assets/css public_html/assets/js public_html/images public_html/icons public_html/index.html
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

func Routes(router *http.ServeMux, cfg *config.ServerConfig) {
	logger := log.WithGroup("server")
	logger.Info("adding routes")

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
		htmlPath := cfg.HTMLPath
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

	// If serving user files then add the handler
	privateFilesPath := cfg.PrivateFilesPath
	if privateFilesPath != "" {
		router.HandleFunc("GET /private-files/", middleware.WebAuth(func(w http.ResponseWriter, r *http.Request) {
			serveFiles(w, r, "/private-files/", privateFilesPath)
		}))
	}

	publicFilesPath := cfg.PublicFilesPath
	if publicFilesPath != "" {
		router.HandleFunc("GET /public-files/", func(w http.ResponseWriter, r *http.Request) {
			serveFiles(w, r, "/public-files/", publicFilesPath)
		})
	}

	// Serve agent files
	router.HandleFunc("GET /agents/{agent_file}", func(w http.ResponseWriter, r *http.Request) {
		fileName := r.PathValue("agent_file")

		if strings.Contains(fileName, "..") {
			http.Error(w, "Invalid file name", http.StatusBadRequest)
			return
		}

		agentPath := cfg.AgentPath
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
	router.HandleFunc("GET /api/icons", middleware.WebAuth(HandleGetIcons))
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

	if cfg.ListenTunnel != "" {
		router.HandleFunc("GET /tunnels", middleware.WebAuth(checkPermissionUseTunnels(HandleSimplePage)))
	}

	if database.GetInstance().HasAuditLog() {
		router.HandleFunc("GET /audit-logs", middleware.WebAuth(checkPermissionViewAuditLogs(HandleSimplePage)))
	}

	router.HandleFunc("GET /logs/{space_id}/stream", middleware.ApiAuth(HandleLogsStream))
	router.HandleFunc("GET /space-io/{space_id}/run", middleware.ApiAuth(middleware.ApiPermissionRunCommands(HandleRunCommandStream)))
	router.HandleFunc("GET /space-io/{space_id}/copy", middleware.ApiAuth(middleware.ApiPermissionCopyFiles(HandleCopyFileStream)))

	router.HandleFunc("GET /cluster-info", middleware.WebAuth(checkPermissionViewClusterInfo(HandleSimplePage)))

	// Routes without authentication
	if !middleware.HasUsers && cfg.Origin.Server == "" && cfg.Origin.Token == "" {
		router.HandleFunc("GET /initial-system-setup", HandleInitialSystemSetupPage)
	}
	router.HandleFunc("GET /login", HandleLoginPage)
	router.HandleFunc("GET /oauth/grant", middleware.WebAuth(HandleOAuth2GrantPage))
	router.HandleFunc("POST /oauth/grant", middleware.WebAuth(oauth2.HandleGrant))

	// If download path set then enable serving of the download folder
	downloadPath := cfg.DownloadPath
	if downloadPath != "" {
		logger.Info("enabling download endpoint, source folder", "downloadPath", downloadPath)

		router.HandleFunc("GET /download/", func(w http.ResponseWriter, r *http.Request) {
			filePath := r.URL.Path[len("/download/"):]

			if strings.Contains(filePath, "..") {
				http.Error(w, "Invalid file name", http.StatusBadRequest)
				return
			}

			http.ServeFile(w, r, filepath.Join(downloadPath, filePath))
		})
	}

	if cfg.TOTP.Enabled {
		router.HandleFunc("GET /qrcode/{code}", middleware.WebAuth(HandleCreateQRCode))
	}
}

func serveFiles(w http.ResponseWriter, r *http.Request, urlBase string, filesPath string) {
	// Serve index.html if path is empty
	fileName := strings.TrimPrefix(r.URL.Path, urlBase)
	if strings.HasSuffix(fileName, "/") || fileName == "" {
		fileName = fileName + "index.html"
	}

	if strings.Contains(fileName, "..") {
		http.Error(w, "Invalid file name", http.StatusBadRequest)
		return
	}

	// Add headers to allow caching for 4 hours
	w.Header().Set("Cache-Control", "public, max-age=14400")

	// If the file does exist then return a 404
	info, err := os.Stat(filepath.Join(filesPath, fileName))
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
	fs := http.StripPrefix(urlBase, http.FileServer(http.Dir(filesPath)))
	fs.ServeHTTP(w, r)
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

	user := r.Context().Value("user").(*model.User)
	canUseSpaces := user != nil && (user.HasPermission(model.PermissionUseSpaces) || user.HasPermission(model.PermissionManageSpaces))

	w.WriteHeader(http.StatusForbidden)
	err = tmpl.Execute(w, map[string]interface{}{
		"version":             build.Version,
		"permissionUseSpaces": canUseSpaces,
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

// Initialize a new template
func newTemplate(name string) (*template.Template, error) {
	cfg := config.GetServerConfig()

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
		"quote": func(s string) string {
			return strings.ReplaceAll(s, `"`, `\"`)
		},
	}

	// If server.template_path is given then serve the files from that path otherwise serve the embedded files
	templatePath := cfg.TemplatePath
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
	cfg := config.GetServerConfig()

	withDownloads := false
	downloadPath := cfg.DownloadPath
	if downloadPath != "" {
		withDownloads = true
	}

	return user, map[string]interface{}{
		"username":                  user.Username,
		"user_id":                   user.Id,
		"user_email":                user.Email,
		"preferredShell":            user.PreferredShell,
		"user_email_md5":            fmt.Sprintf("%x", md5.Sum([]byte(user.Email))),
		"withDownloads":             withDownloads,
		"hideSupportLinks":          cfg.UI.HideSupportLinks,
		"hideAPITokens":             cfg.UI.HideAPITokens,
		"useGravatar":               cfg.UI.EnableGravatar,
		"permissionManageUsers":     user.HasPermission(model.PermissionManageUsers),
		"permissionManageGroups":    user.HasPermission(model.PermissionManageGroups),
		"permissionManageRoles":     user.HasPermission(model.PermissionManageRoles),
		"permissionManageTemplates": user.HasPermission(model.PermissionManageTemplates),
		"permissionManageVariables": user.HasPermission(model.PermissionManageVariables),
		"permissionManageSpaces":    user.HasPermission(model.PermissionManageSpaces),
		"permissionManageVolumes":   user.HasPermission(model.PermissionManageVolumes),
		"permissionUseSpaces":       user.HasPermission(model.PermissionUseSpaces) || user.HasPermission(model.PermissionManageSpaces),
		"permissionUseTunnels":      user.HasPermission(model.PermissionUseTunnels) && cfg.ListenTunnel != "",
		"permissionViewAuditLogs":   user.HasPermission(model.PermissionViewAuditLogs) && database.GetInstance().HasAuditLog(),
		"permissionTransferSpaces":  user.HasPermission(model.PermissionTransferSpaces),
		"permissionShareSpaces":     user.HasPermission(model.PermissionShareSpaces),
		"permissionViewClusterInfo": user.HasPermission(model.PermissionClusterInfo) && cfg.Cluster.AdvertiseAddr != "",
		"permissionUseVNC":          user.HasPermission(model.PermissionUseVNC) || cfg.LeafNode,
		"permissionUseWebTerminal":  user.HasPermission(model.PermissionUseWebTerminal) || cfg.LeafNode,
		"permissionUseSSH":          user.HasPermission(model.PermissionUseSSH) || cfg.LeafNode,
		"permissionUseCodeServer":   user.HasPermission(model.PermissionUseCodeServer) || cfg.LeafNode,
		"permissionUseVSCodeTunnel": user.HasPermission(model.PermissionUseVSCodeTunnel) || cfg.LeafNode,
		"permissionUseLogs":         user.HasPermission(model.PermissionUseLogs) || cfg.LeafNode,
		"permissionRunCommands":     user.HasPermission(model.PermissionRunCommands) || cfg.LeafNode,
		"permissionCopyFiles":       user.HasPermission(model.PermissionCopyFiles) || cfg.LeafNode,
		"permissionUseMCPServer":    user.HasPermission(model.PermissionUseMCPServer),
		"permissionUseWebAssistant": user.HasPermission(model.PermissionUseWebAssistant),
		"version":                   build.Version,
		"buildDate":                 build.Date,
		"zone":                      cfg.Zone,
		"timezone":                  cfg.Timezone,
		"disableSpaceCreate":        cfg.DisableSpaceCreate,
		"totpEnabled":               cfg.TOTP.Enabled,
		"clusterMode":               cfg.Cluster.AdvertiseAddr != "",
		"isLeafNode":                cfg.LeafNode,
		"logoURL":                   cfg.UI.LogoURL,
		"logoInvert":                cfg.UI.LogoInvert,
		"aiChatEnabled":             cfg.Chat.Enabled,
		"aiChatStyle":               cfg.Chat.UIStyle,
	}
}
