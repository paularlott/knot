package web

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"

	"github.com/pelletier/go-toml/v2"
)

// Icon represents a single icon entry
type Icon struct {
	Description string `toml:"description" json:"description"`
	Url         string `toml:"url" json:"url"`
}

type IconList struct {
	Icons []Icon `toml:"icons"`
}

var (
	defaultIcons = []Icon{
		{Description: "Adminer (built-in)", Url: "/icons/adminer.svg"},
		{Description: "Alma Linux (built-in)", Url: "/icons/alma-linux.svg"},
		{Description: "Ansible (built-in)", Url: "/icons/ansible.svg"},
		{Description: "Apache (built-in)", Url: "/icons/apache.svg"},
		{Description: "Apple (built-in)", Url: "/icons/apple.svg"},
		{Description: "Arch Linux (built-in)", Url: "/icons/arch-linux.svg"},
		{Description: "C (built-in)", Url: "/icons/c.svg"},
		{Description: "Caddy (built-in)", Url: "/icons/caddy.svg"},
		{Description: "CentOS (built-in)", Url: "/icons/centos.svg"},
		{Description: "Chrome (built-in)", Url: "/icons/chrome.svg"},
		{Description: "Chromium (built-in)", Url: "/icons/chromium.svg"},
		{Description: "CoreOS (built-in)", Url: "/icons/coreos.svg"},
		{Description: "CouchDB (built-in)", Url: "/icons/couchdb.svg"},
		{Description: "C++ (built-in)", Url: "/icons/cpp.svg"},
		{Description: "C# (built-in)", Url: "/icons/csharp.svg"},
		{Description: "CSS3 (built-in)", Url: "/icons/css3.svg"},
		{Description: "Debian Linux (built-in)", Url: "/icons/debian-linux.svg"},
		{Description: "Docker (built-in)", Url: "/icons/docker.svg"},
		{Description: "Elastic (built-in)", Url: "/icons/elastic.svg"},
		{Description: "Electron (built-in)", Url: "/icons/electron.svg"},
		{Description: "Erlang (built-in)", Url: "/icons/erlang.svg"},
		{Description: "Fedora (built-in)", Url: "/icons/fedora.svg"},
		{Description: "Files (built-in)", Url: "/icons/files.svg"},
		{Description: "Fortran (built-in)", Url: "/icons/fortran.svg"},
		{Description: "Go (built-in)", Url: "/icons/go.svg"},
		{Description: "Golang (built-in)", Url: "/icons/golang.svg"},
		{Description: "Grafana (built-in)", Url: "/icons/grafana.svg"},
		{Description: "HTML5 (built-in)", Url: "/icons/html5.svg"},
		{Description: "Java (built-in)", Url: "/icons/java.svg"},
		{Description: "JavaScript (built-in)", Url: "/icons/javascript.svg"},
		{Description: "Laravel (built-in)", Url: "/icons/laravel.svg"},
		{Description: "Linux Mint (built-in)", Url: "/icons/linux-mint.svg"},
		{Description: "Linux (built-in)", Url: "/icons/linux.svg"},
		{Description: "Lua (built-in)", Url: "/icons/lua.svg"},
		{Description: "Mailpit (built-in)", Url: "/icons/mailpit.svg"},
		{Description: "MariaDB (built-in)", Url: "/icons/mariadb.svg"},
		{Description: "Markdown (built-in)", Url: "/icons/markdown.svg"},
		{Description: "MongoDB (built-in)", Url: "/icons/mongodb.svg"},
		{Description: "MySQL (built-in)", Url: "/icons/mysql.svg"},
		{Description: "Next.js (built-in)", Url: "/icons/nextjs.svg"},
		{Description: "Nginx Proxy Manager (built-in)", Url: "/icons/nginx-proxy-manager.svg"},
		{Description: "Nginx (built-in)", Url: "/icons/nginx.svg"},
		{Description: "NixOS (built-in)", Url: "/icons/nixos.svg"},
		{Description: "Node.js (built-in)", Url: "/icons/nodejs.svg"},
		{Description: "npm (built-in)", Url: "/icons/npm.svg"},
		{Description: "openSUSE (built-in)", Url: "/icons/opensuse.svg"},
		{Description: "Oracle (built-in)", Url: "/icons/oracle.svg"},
		{Description: "pgAdmin (built-in)", Url: "/icons/pgadmin.svg"},
		{Description: "PHP (built-in)", Url: "/icons/php.svg"},
		{Description: "phpMyAdmin (built-in)", Url: "/icons/phpmyadmin.svg"},
		{Description: "PhpStorm (built-in)", Url: "/icons/phpstorm.svg"},
		{Description: "Podman (built-in)", Url: "/icons/podman.svg"},
		{Description: "PostgreSQL (built-in)", Url: "/icons/postgres.svg"},
		{Description: "Proxmox (built-in)", Url: "/icons/proxmox.svg"},
		{Description: "r (built-in)", Url: "/icons/r.svg"},
		{Description: "RabbitMQ (built-in)", Url: "/icons/rabbitmq.svg"},
		{Description: "Rails (built-in)", Url: "/icons/rails-plain.svg"},
		{Description: "Raspberry Pi (built-in)", Url: "/icons/raspberry-pi.svg"},
		{Description: "React.js (built-in)", Url: "/icons/reactjs.svg"},
		{Description: "Red Hat Linux (built-in)", Url: "/icons/redhat-linux.svg"},
		{Description: "Redis (built-in)", Url: "/icons/redis.svg"},
		{Description: "Router (built-in)", Url: "/icons/router.svg"},
		{Description: "Ruby (built-in)", Url: "/icons/ruby.svg"},
		{Description: "Rust (built-in)", Url: "/icons/rust.svg"},
		{Description: "SQLite Browser (built-in)", Url: "/icons/sqlitebrowser.svg"},
		{Description: "Terminal (built-in)", Url: "/icons/terminal.svg"},
		{Description: "Terminal (built-in)", Url: "/icons/nexterm.svg"},
		{Description: "Terraform (built-in)", Url: "/icons/terraform.svg"},
		{Description: "Ubuntu Linux (built-in)", Url: "/icons/ubuntu-linux-alt.svg"},
		{Description: "Unraid (built-in)", Url: "/icons/unraid.svg"},
		{Description: "Valkey (built-in)", Url: "/icons/valkey.svg"},
		{Description: "Vite (built-in)", Url: "/icons/vite.svg"},
		{Description: "VMware (built-in)", Url: "/icons/vmware.svg"},
		{Description: "VSCode (built-in)", Url: "/icons/vscode.svg"},
		{Description: "WebHook (built-in)", Url: "/icons/webhook.svg"},
		{Description: "WordPress (built-in)", Url: "/icons/wordpress.svg"},
		{Description: "WWW (built-in)", Url: "/icons/www.svg"},
		{Description: "X (built-in)", Url: "/icons/x.svg"},
		{Description: "XCP-ng (built-in)", Url: "/icons/xcp-ng.svg"},
		{Description: "Zig (built-in)", Url: "/icons/zig.svg"},
	}
)

func loadIcons() []Icon {
	var iconList []Icon

	// Load the default icons
	iconList = append(iconList, defaultIcons...)

	iconFiles := viper.GetStringSlice("server.ui.icons")
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
			iconsFromFile.Icons[i].Description = iconsFromFile.Icons[i].Description + " (" + description + ")"
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
