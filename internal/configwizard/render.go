package configwizard

import (
	_ "embed"
	"html/template"
	"net/http"

	"github.com/paularlott/knot/internal/log"
)

//go:embed templates/wizard.html.tmpl
var wizardHTML string

//go:embed templates/success.html.tmpl
var successHTML string

type wizardView struct {
	Form         Form
	ConfigPath   string
	ConfigExists bool
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

func renderWizard(w http.ResponseWriter, form Form, configPath string, configExists bool) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl, err := htmlTemplate(wizardHTML)
	if err != nil {
		log.Error("parsing wizard template", "err", err)
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	if err := tmpl.Execute(w, wizardView{Form: form, ConfigPath: configPath, ConfigExists: configExists}); err != nil {
		log.Error("executing wizard template", "err", err)
	}
}

func renderSuccess(w http.ResponseWriter, configPath string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl, err := htmlTemplate(successHTML)
	if err != nil {
		log.Error("parsing success template", "err", err)
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	if err := tmpl.Execute(w, map[string]string{"Path": configPath}); err != nil {
		log.Error("executing success template", "err", err)
	}
}
