package model

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/template"

	"github.com/paularlott/knot/internal/config"
)

// Parse an input string and resolve knot variables
func ResolveVariables(srcString string, t *Template, space *Space, user *User, variables map[string]interface{}) (string, error) {

	// If no variables are provided then create an empty map
	if variables == nil {
		variables = map[string]interface{}{}
	}

	// Build map of custom space variables and any that are in the template and not space add as blanks
	var custVars = make(map[string]interface{})
	for _, field := range space.CustomFields {
		custVars[field.Name] = field.Value
	}
	for _, field := range t.CustomFields {
		if _, ok := custVars[field.Name]; !ok {
			custVars[field.Name] = ""
		}
	}

	// Add a function to the template engine
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
		"toUpper": strings.ToUpper,
		"toLower": strings.ToLower,
		"json": func(v interface{}) string {
			b, _ := json.Marshal(v)
			return string(b)
		},
	}

	// Passe the YAML string through the template engine to resolve variables
	tmpl, err := template.New("tmpl").Funcs(funcs).Delims("${{", "}}").Parse(srcString)
	if err != nil {
		return srcString, err
	}

	cfg := config.GetServerConfig()

	// Get the wildcard domain without the *
	wildcardDomain := cfg.WildcardDomain
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
			"url":             strings.TrimSuffix(cfg.URL, "/"),
			"agent_endpoint":  cfg.AgentEndpoint,
			"wildcard_domain": wildcardDomain,
			"zone":            cfg.Zone,
			"timezone":        cfg.Timezone,
		},
		"nomad": map[string]interface{}{
			"dc":     os.Getenv("NOMAD_DC"),
			"region": os.Getenv("NOMAD_REGION"),
		},
		"var":    variables,
		"custom": &custVars,
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
	cfg := config.GetServerConfig()

	// Filter the variables, local takes precedence, then variables with zone matching the server, then global
	filteredVars := make(map[string]*TemplateVar, len(variables))
	for _, variable := range variables {
		if variable.IsDeleted {
			continue
		}

		allowVar := len(variable.Zones) == 0
		zoneMatch := false
		if !allowVar {
			// Allow if any zone matches the local zone
			for _, zone := range variable.Zones {
				if zone == cfg.Zone {
					allowVar = true
					break
				}
			}

			// Allow if all negated zones do not match
			if !allowVar {
				hasNegated := false
				allNegatedDontMatch := true
				for _, zone := range variable.Zones {
					if strings.HasPrefix(zone, "!") {
						hasNegated = true
						if zone[1:] == cfg.Zone {
							allNegatedDontMatch = false
							break
						}
					}
				}
				if hasNegated && allNegatedDontMatch {
					allowVar = true
				}
			}
		}

		if !allowVar {
			continue
		}

		// Test if variable already in the list
		existing, ok := filteredVars[variable.Name]
		if ok {
			if existing.Local || (len(existing.Zones) == 0 && !zoneMatch && !variable.Local) || (len(existing.Zones) != 0 && !variable.Local) {
				continue
			}
		}

		filteredVars[variable.Name] = variable
	}

	vars := make(map[string]interface{}, len(filteredVars))
	for _, variable := range filteredVars {
		vars[variable.Name] = variable.Value
	}

	return vars
}
