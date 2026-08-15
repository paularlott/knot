package configwizard

import (
	_ "embed"
	"html/template"
	"net/http"

	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/util"
)

//go:embed templates/wizard.html.tmpl
var wizardHTML string

//go:embed templates/success.html.tmpl
var successHTML string

type wizardView struct {
	Form           Form
	ConfigPath     string
	ConfigExists   bool
	Desktop        bool
	EditorWritable bool
	HostIPToken    string
	BasePath       string
	// Editing is set when the wizard pre-filled from an existing config;
	// the deployment question is skipped because the config answers it.
	Editing bool
}

func htmlTemplate(src string) (*template.Template, error) {
	return template.New("page").Funcs(template.FuncMap{
		"checked": func(b bool) string {
			if b {
				return "checked"
			}
			return ""
		},
		"dbSelected": func(formType, option string) string {
			if formType == option {
				return "checked"
			}
			return ""
		},
	}).Parse(src)
}

func renderWizard(w http.ResponseWriter, form Form, configPath string, configExists bool, o Options) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl, err := htmlTemplate(wizardHTML)
	if err != nil {
		log.Error("parsing wizard template", "err", err)
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	view := wizardView{Form: form, ConfigPath: configPath, ConfigExists: configExists, HostIPToken: util.HostIPToken, BasePath: o.BasePath}
	if o.BasePath == "" {
		view.BasePath = "/"
	}
	if o.Desktop {
		view.Desktop = true
	}
	// In desktop overwrite mode the editor starts editable even though a
	// (partial) config exists.
	view.EditorWritable = !configExists || o.AllowOverwrite
	view.Editing = configExists
	if err := tmpl.Execute(w, view); err != nil {
		log.Error("executing wizard template", "err", err)
	}
}

func renderSuccess(w http.ResponseWriter, configPath string, o Options) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl, err := htmlTemplate(successHTML)
	if err != nil {
		log.Error("parsing success template", "err", err)
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	basePath := o.BasePath
	if basePath == "" {
		basePath = "/"
	}
	data := map[string]interface{}{"Path": configPath, "Desktop": o.Desktop, "BasePath": basePath}
	if err := tmpl.Execute(w, data); err != nil {
		log.Error("executing success template", "err", err)
	}
}
