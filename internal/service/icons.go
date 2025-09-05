package service

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/paularlott/knot/internal/config"
	"github.com/rs/zerolog/log"

	"github.com/BurntSushi/toml"
)

// Icon represents a single icon entry
type Icon struct {
	Description string `toml:"description" json:"description"`
	Source      string `toml:"-" json:"source"`
	URL         string `toml:"url" json:"url"`
}

type IconList struct {
	Icons []Icon `toml:"icons" json:"icons"`
}

var (
	iconService *IconService
	iconOnce    sync.Once
)

type IconService struct {
	icons []Icon
	mutex sync.RWMutex
}

// GetIconService returns the singleton icon service instance
func GetIconService() *IconService {
	iconOnce.Do(func() {
		iconService = &IconService{}
		iconService.loadIcons()
	})
	return iconService
}

// GetIcons returns all available icons (built-in and user-supplied)
func (s *IconService) GetIcons() []Icon {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	// Return a copy to prevent external modification
	result := make([]Icon, len(s.icons))
	copy(result, s.icons)
	return result
}

// ReloadIcons reloads icons from configuration
func (s *IconService) ReloadIcons() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.loadIcons()
}

// loadIcons loads icons from built-in list and configured files
func (s *IconService) loadIcons() {
	var iconList []Icon

	cfg := config.GetServerConfig()

	if cfg.UI.EnableBuiltinIcons {
		// Load the default icons
		iconList = append(iconList, getDefaultIcons()...)
	}

	iconFiles := cfg.UI.Icons
	for _, iconFile := range iconFiles {
		log.Info().Msgf("Loading icons from file: %s", iconFile)

		// If file doesn't exist, skip it
		_, err := os.Stat(iconFile)
		if err != nil {
			log.Warn().Msgf("Icon file %s does not exist, skipping", iconFile)
			continue
		}

		// Load the icons from the .toml file
		file, err := os.Open(iconFile)
		if err != nil {
			log.Warn().Msgf("Failed to open icon file %s: %v", iconFile, err)
			continue
		}

		// Read the data from the file
		iconData, err := os.ReadFile(iconFile)
		if err != nil {
			log.Warn().Msgf("Failed to read icon file %s: %v", iconFile, err)
			file.Close()
			continue
		}

		file.Close()

		var iconsFromFile IconList
		if err := toml.Unmarshal(iconData, &iconsFromFile); err != nil {
			log.Warn().Msgf("Failed to unmarshal icons from %s: %v", iconFile, err)
			continue
		}

		// Extract the filename without extension
		filename := filepath.Base(iconFile)
		ext := filepath.Ext(filename)
		description := strings.TrimSuffix(filename, ext)

		// Add (filename) to all descriptions
		for i := range iconsFromFile.Icons {
			iconsFromFile.Icons[i].Source = description
			iconList = append(iconList, iconsFromFile.Icons[i])
		}
	}

	// Sort alphabetically by description
	if len(iconList) > 0 {
		sort.Slice(iconList, func(i, j int) bool {
			return iconList[i].Description < iconList[j].Description
		})
	}

	s.icons = iconList
}

// getDefaultIcons returns the built-in icon list
func getDefaultIcons() []Icon {
	return []Icon{
		{Description: "Adminer", Source: "built-in", URL: "/icons/adminer.svg"},
		{Description: "Alma Linux", Source: "built-in", URL: "/icons/alma-linux.svg"},
		{Description: "Alpine Linux", Source: "built-in", URL: "/icons/alpine-linux.svg"},
		{Description: "Ansible", Source: "built-in", URL: "/icons/ansible.svg"},
		{Description: "Apache", Source: "built-in", URL: "/icons/apache.svg"},
		{Description: "Apple", Source: "built-in", URL: "/icons/apple.svg"},
		{Description: "Arch Linux", Source: "built-in", URL: "/icons/arch-linux.svg"},
		{Description: "C", Source: "built-in", URL: "/icons/c.svg"},
		{Description: "Caddy", Source: "built-in", URL: "/icons/caddy.svg"},
		{Description: "CentOS", Source: "built-in", URL: "/icons/centos.svg"},
		{Description: "Chrome", Source: "built-in", URL: "/icons/chrome.svg"},
		{Description: "Chromium", Source: "built-in", URL: "/icons/chromium.svg"},
		{Description: "CouchDB", Source: "built-in", URL: "/icons/couchdb.svg"},
		{Description: "C++", Source: "built-in", URL: "/icons/cpp.svg"},
		{Description: "C#", Source: "built-in", URL: "/icons/csharp.svg"},
		{Description: "CSS3", Source: "built-in", URL: "/icons/css3.svg"},
		{Description: "Debian Linux", Source: "built-in", URL: "/icons/debian-linux.svg"},
		{Description: "Docker", Source: "built-in", URL: "/icons/docker.svg"},
		{Description: "Elastic", Source: "built-in", URL: "/icons/elastic.svg"},
		{Description: "Electron", Source: "built-in", URL: "/icons/electron.svg"},
		{Description: "Erlang", Source: "built-in", URL: "/icons/erlang.svg"},
		{Description: "Fedora", Source: "built-in", URL: "/icons/fedora.svg"},
		{Description: "Files", Source: "built-in", URL: "/icons/files.svg"},
		{Description: "Fortran", Source: "built-in", URL: "/icons/fortran.svg"},
		{Description: "Go", Source: "built-in", URL: "/icons/go.svg"},
		{Description: "Golang", Source: "built-in", URL: "/icons/golang.svg"},
		{Description: "Grafana", Source: "built-in", URL: "/icons/grafana.svg"},
		{Description: "HTML5", Source: "built-in", URL: "/icons/html5.svg"},
		{Description: "Java", Source: "built-in", URL: "/icons/java.svg"},
		{Description: "JavaScript", Source: "built-in", URL: "/icons/javascript.svg"},
		{Description: "Laravel", Source: "built-in", URL: "/icons/laravel.svg"},
		{Description: "Linux Mint", Source: "built-in", URL: "/icons/linux-mint.svg"},
		{Description: "Linux", Source: "built-in", URL: "/icons/linux.svg"},
		{Description: "Lua", Source: "built-in", URL: "/icons/lua.svg"},
		{Description: "Mailpit", Source: "built-in", URL: "/icons/mailpit.svg"},
		{Description: "MariaDB", Source: "built-in", URL: "/icons/mariadb.svg"},
		{Description: "Markdown", Source: "built-in", URL: "/icons/markdown.svg"},
		{Description: "MongoDB", Source: "built-in", URL: "/icons/mongodb.svg"},
		{Description: "MySQL", Source: "built-in", URL: "/icons/mysql.svg"},
		{Description: "Nexterm", Source: "built-in", URL: "/icons/nexterm.svg"},
		{Description: "Next.js", Source: "built-in", URL: "/icons/nextjs.svg"},
		{Description: "Nginx Proxy Manager", Source: "built-in", URL: "/icons/nginx-proxy-manager.svg"},
		{Description: "Nginx", Source: "built-in", URL: "/icons/nginx.svg"},
		{Description: "Node.js", Source: "built-in", URL: "/icons/nodejs.svg"},
		{Description: "npm", Source: "built-in", URL: "/icons/npm.svg"},
		{Description: "openSUSE", Source: "built-in", URL: "/icons/opensuse.svg"},
		{Description: "Oracle", Source: "built-in", URL: "/icons/oracle.svg"},
		{Description: "pgAdmin", Source: "built-in", URL: "/icons/pgadmin.svg"},
		{Description: "PHP", Source: "built-in", URL: "/icons/php.svg"},
		{Description: "phpMyAdmin", Source: "built-in", URL: "/icons/phpmyadmin.svg"},
		{Description: "Podman", Source: "built-in", URL: "/icons/podman.svg"},
		{Description: "PostgreSQL", Source: "built-in", URL: "/icons/postgres.svg"},
		{Description: "Proxmox", Source: "built-in", URL: "/icons/proxmox.svg"},
		{Description: "R", Source: "built-in", URL: "/icons/r.svg"},
		{Description: "RabbitMQ", Source: "built-in", URL: "/icons/rabbitmq.svg"},
		{Description: "Rails", Source: "built-in", URL: "/icons/rails-plain.svg"},
		{Description: "Raspberry Pi", Source: "built-in", URL: "/icons/raspberry-pi.svg"},
		{Description: "React.js", Source: "built-in", URL: "/icons/reactjs.svg"},
		{Description: "Red Hat Linux", Source: "built-in", URL: "/icons/redhat-linux.svg"},
		{Description: "Redis", Source: "built-in", URL: "/icons/redis.svg"},
		{Description: "Router", Source: "built-in", URL: "/icons/router.svg"},
		{Description: "Ruby", Source: "built-in", URL: "/icons/ruby.svg"},
		{Description: "Rust", Source: "built-in", URL: "/icons/rust.svg"},
		{Description: "SQLite Browser", Source: "built-in", URL: "/icons/sqlitebrowser.svg"},
		{Description: "Terminal", Source: "built-in", URL: "/icons/terminal.svg"},
		{Description: "Terraform", Source: "built-in", URL: "/icons/terraform.svg"},
		{Description: "Ubuntu Linux", Source: "built-in", URL: "/icons/ubuntu-linux-alt.svg"},
		{Description: "Unraid", Source: "built-in", URL: "/icons/unraid.svg"},
		{Description: "Valkey", Source: "built-in", URL: "/icons/valkey.svg"},
		{Description: "Vite", Source: "built-in", URL: "/icons/vite.svg"},
		{Description: "VSCode", Source: "built-in", URL: "/icons/vscode.svg"},
		{Description: "WebHook", Source: "built-in", URL: "/icons/webhook.svg"},
		{Description: "WordPress", Source: "built-in", URL: "/icons/wordpress.svg"},
		{Description: "WWW", Source: "built-in", URL: "/icons/www.svg"},
		{Description: "XCP-ng", Source: "built-in", URL: "/icons/xcp-ng.svg"},
		{Description: "Zig", Source: "built-in", URL: "/icons/zig.svg"},
	}
}
