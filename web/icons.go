package web

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/paularlott/knot/internal/config"
	"github.com/rs/zerolog/log"

	"github.com/BurntSushi/toml"
)

// Icon represents a single icon entry
type Icon struct {
	Description string `toml:"description" json:"description"`
	Source      string `toml:"-" json:"source"`
	Url         string `toml:"url" json:"url"`
}

type IconList struct {
	Icons []Icon `toml:"icons"`
}

var (
	defaultIcons = []Icon{
		{Description: "Adminer", Source: "built-in", Url: "/icons/adminer.svg"},
		{Description: "Alma Linux", Source: "built-in", Url: "/icons/alma-linux.svg"},
		{Description: "Ansible", Source: "built-in", Url: "/icons/ansible.svg"},
		{Description: "Apache", Source: "built-in", Url: "/icons/apache.svg"},
		{Description: "Apple", Source: "built-in", Url: "/icons/apple.svg"},
		{Description: "Arch Linux", Source: "built-in", Url: "/icons/arch-linux.svg"},
		{Description: "C", Source: "built-in", Url: "/icons/c.svg"},
		{Description: "Caddy", Source: "built-in", Url: "/icons/caddy.svg"},
		{Description: "CentOS", Source: "built-in", Url: "/icons/centos.svg"},
		{Description: "Chrome", Source: "built-in", Url: "/icons/chrome.svg"},
		{Description: "Chromium", Source: "built-in", Url: "/icons/chromium.svg"},
		{Description: "CouchDB", Source: "built-in", Url: "/icons/couchdb.svg"},
		{Description: "C++", Source: "built-in", Url: "/icons/cpp.svg"},
		{Description: "C#", Source: "built-in", Url: "/icons/csharp.svg"},
		{Description: "CSS3", Source: "built-in", Url: "/icons/css3.svg"},
		{Description: "Debian Linux", Source: "built-in", Url: "/icons/debian-linux.svg"},
		{Description: "Desktop", Source: "built-in", Url: "/icons/desktop.svg"},
		{Description: "Docker", Source: "built-in", Url: "/icons/docker.svg"},
		{Description: "Elastic", Source: "built-in", Url: "/icons/elastic.svg"},
		{Description: "Electron", Source: "built-in", Url: "/icons/electron.svg"},
		{Description: "Erlang", Source: "built-in", Url: "/icons/erlang.svg"},
		{Description: "Fedora", Source: "built-in", Url: "/icons/fedora.svg"},
		{Description: "Files", Source: "built-in", Url: "/icons/files.svg"},
		{Description: "Fortran", Source: "built-in", Url: "/icons/fortran.svg"},
		{Description: "Go", Source: "built-in", Url: "/icons/go.svg"},
		{Description: "Golang", Source: "built-in", Url: "/icons/golang.svg"},
		{Description: "Grafana", Source: "built-in", Url: "/icons/grafana.svg"},
		{Description: "HTML5", Source: "built-in", Url: "/icons/html5.svg"},
		{Description: "Java", Source: "built-in", Url: "/icons/java.svg"},
		{Description: "JavaScript", Source: "built-in", Url: "/icons/javascript.svg"},
		{Description: "Laravel", Source: "built-in", Url: "/icons/laravel.svg"},
		{Description: "Linux Mint", Source: "built-in", Url: "/icons/linux-mint.svg"},
		{Description: "Linux", Source: "built-in", Url: "/icons/linux.svg"},
		{Description: "Lua", Source: "built-in", Url: "/icons/lua.svg"},
		{Description: "Mailpit", Source: "built-in", Url: "/icons/mailpit.svg"},
		{Description: "MariaDB", Source: "built-in", Url: "/icons/mariadb.svg"},
		{Description: "Markdown", Source: "built-in", Url: "/icons/markdown.svg"},
		{Description: "MongoDB", Source: "built-in", Url: "/icons/mongodb.svg"},
		{Description: "MySQL", Source: "built-in", Url: "/icons/mysql.svg"},
		{Description: "Next.js", Source: "built-in", Url: "/icons/nextjs.svg"},
		{Description: "Nginx Proxy Manager", Source: "built-in", Url: "/icons/nginx-proxy-manager.svg"},
		{Description: "Nginx", Source: "built-in", Url: "/icons/nginx.svg"},
		{Description: "Node.js", Source: "built-in", Url: "/icons/nodejs.svg"},
		{Description: "npm", Source: "built-in", Url: "/icons/npm.svg"},
		{Description: "openSUSE", Source: "built-in", Url: "/icons/opensuse.svg"},
		{Description: "Oracle", Source: "built-in", Url: "/icons/oracle.svg"},
		{Description: "pgAdmin", Source: "built-in", Url: "/icons/pgadmin.svg"},
		{Description: "PHP", Source: "built-in", Url: "/icons/php.svg"},
		{Description: "phpMyAdmin", Source: "built-in", Url: "/icons/phpmyadmin.svg"},
		{Description: "Podman", Source: "built-in", Url: "/icons/podman.svg"},
		{Description: "PostgreSQL", Source: "built-in", Url: "/icons/postgres.svg"},
		{Description: "Proxmox", Source: "built-in", Url: "/icons/proxmox.svg"},
		{Description: "R", Source: "built-in", Url: "/icons/r.svg"},
		{Description: "RabbitMQ", Source: "built-in", Url: "/icons/rabbitmq.svg"},
		{Description: "Rails", Source: "built-in", Url: "/icons/rails-plain.svg"},
		{Description: "Raspberry Pi", Source: "built-in", Url: "/icons/raspberry-pi.svg"},
		{Description: "React.js", Source: "built-in", Url: "/icons/reactjs.svg"},
		{Description: "Red Hat Linux", Source: "built-in", Url: "/icons/redhat-linux.svg"},
		{Description: "Redis", Source: "built-in", Url: "/icons/redis.svg"},
		{Description: "Router", Source: "built-in", Url: "/icons/router.svg"},
		{Description: "Ruby", Source: "built-in", Url: "/icons/ruby.svg"},
		{Description: "Rust", Source: "built-in", Url: "/icons/rust.svg"},
		{Description: "SQLite Browser", Source: "built-in", Url: "/icons/sqlitebrowser.svg"},
		{Description: "Terminal", Source: "built-in", Url: "/icons/terminal.svg"},
		{Description: "Terminal", Source: "built-in", Url: "/icons/nexterm.svg"},
		{Description: "Terraform", Source: "built-in", Url: "/icons/terraform.svg"},
		{Description: "Ubuntu Linux", Source: "built-in", Url: "/icons/ubuntu-linux-alt.svg"},
		{Description: "Unraid", Source: "built-in", Url: "/icons/unraid.svg"},
		{Description: "Valkey", Source: "built-in", Url: "/icons/valkey.svg"},
		{Description: "Vite", Source: "built-in", Url: "/icons/vite.svg"},
		{Description: "VMware", Source: "built-in", Url: "/icons/vmware.svg"},
		{Description: "VSCode", Source: "built-in", Url: "/icons/vscode.svg"},
		{Description: "WebHook", Source: "built-in", Url: "/icons/webhook.svg"},
		{Description: "WordPress", Source: "built-in", Url: "/icons/wordpress.svg"},
		{Description: "WWW", Source: "built-in", Url: "/icons/www.svg"},
		{Description: "XCP-ng", Source: "built-in", Url: "/icons/xcp-ng.svg"},
		{Description: "Zig", Source: "built-in", Url: "/icons/zig.svg"},
	}
)

func loadIcons() []Icon {
	var iconList []Icon

	cfg := config.GetServerConfig()

	if cfg.UI.EnableBuiltinIcons {
		// Load the default icons
		iconList = append(iconList, defaultIcons...)
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

	return iconList
}
