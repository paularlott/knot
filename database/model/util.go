package model

import (
	"bytes"
	"os"
	"strings"
	"text/template"

	"github.com/paularlott/knot/internal/config"

	"github.com/spf13/viper"
)

// Parse an input string and resolve knot variables
func ResolveVariables(srcString string, t *Template, space *Space, user *User, variables *map[string]interface{}) (string, error) {

	// If no variables are provided then create an empty map
	if variables == nil {
		variables = &map[string]interface{}{}
	}

	// Passe the YAML string through the template engine to resolve variables
	tmpl, err := template.New("tmpl").Delims("${{", "}}").Parse(srcString)
	if err != nil {
		return srcString, err
	}

	// Get the wildcard domain without the *
	wildcardDomain := viper.GetString("server.wildcard_domain")
	if wildcardDomain != "" && wildcardDomain[0] == '*' {
		wildcardDomain = wildcardDomain[1:]
	}

	data := map[string]interface{}{
		"space": map[string]interface{}{
			"id":   "",
			"name": "",
		},
		"template": map[string]interface{}{
			"id":   "",
			"name": "",
		},
		"user": map[string]interface{}{
			"id":               "",
			"username":         "",
			"timezone":         "",
			"email":            "",
			"service_password": "",
		},
		"server": map[string]interface{}{
			"url":             strings.TrimSuffix(viper.GetString("server.url"), "/"),
			"agent_endpoint":  viper.GetString("server.agent_endpoint"),
			"wildcard_domain": wildcardDomain,
			"location":        config.Location,
			"timezone":        config.Timezone,
		},
		"nomad": map[string]interface{}{
			"dc":     os.Getenv("NOMAD_DC"),
			"region": os.Getenv("NOMAD_REGION"),
		},
		"var": variables,
	}

	if space != nil {
		data["space"] = map[string]interface{}{
			"id":   space.Id,
			"name": space.Name,
		}
	}

	if t != nil {
		data["template"] = map[string]interface{}{
			"id":   t.Id,
			"name": t.Name,
		}
	}

	if user != nil {
		data["user"] = map[string]interface{}{
			"id":               user.Id,
			"username":         user.Username,
			"timezone":         user.Timezone,
			"email":            user.Email,
			"service_password": user.ServicePassword,
		}
	}

	var tmplBytes bytes.Buffer
	err = tmpl.Execute(&tmplBytes, data)
	if err != nil {
		return srcString, err
	}

	return tmplBytes.String(), nil
}

func FilterVars(variables []*TemplateVar) map[string]interface{} {
	// Filter the variables, local takes precedence, then variables with location matching the server, then global
	filteredVars := make(map[string]*TemplateVar, len(variables))
	for _, variable := range variables {
		if variable.Location == "" || variable.Location == config.Location {

			// Test if variable already in the list
			existing, ok := filteredVars[variable.Name]
			if ok {
				if existing.Local || (existing.Location != "" && !variable.Local) {
					continue
				}
			}

			filteredVars[variable.Name] = variable
		}
	}

	vars := make(map[string]interface{}, len(filteredVars))
	for _, variable := range filteredVars {
		vars[variable.Name] = variable.Value
	}

	return vars
}
