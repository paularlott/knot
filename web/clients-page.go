package web

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/log"
)

func HandleClientsPage(w http.ResponseWriter, r *http.Request) {
	tmpl, err := newTemplate("page-clients.tmpl")
	if err != nil {
		log.Error(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	} else if tmpl == nil {
		showPageNotFound(w, r)
		return
	}

	_, data := getCommonTemplateData(r)

	// Read the package SHA256
	cfg := config.GetServerConfig()
	var sha256 string

	packagePath := cfg.PackagePath
	if packagePath != "" {
		// Read from disk
		sha256Bytes, err := os.ReadFile(filepath.Join(packagePath, "knot.zip.sha256"))
		if err == nil {
			sha256 = strings.TrimSpace(string(sha256Bytes))
		}
	} else {
		// Read from embedded
		file, err := packageFiles.Open("packages/knot.zip.sha256")
		if err == nil {
			defer file.Close()
			sha256Bytes, err := io.ReadAll(file)
			if err == nil {
				sha256 = strings.TrimSpace(string(sha256Bytes))
			}
		}
	}

	data["knotPackageSha256"] = sha256

	err = tmpl.Execute(w, data)
	if err != nil {
		log.Error(err.Error())
	}
}
