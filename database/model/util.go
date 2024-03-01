package model

import (
	"bytes"
	"text/template"

	"github.com/spf13/viper"
)

// Parse an input string and resolve knot variables
func ResolveVariables(srcString string, t *Template, space *Space, user *User, variables *map[string]interface{}) (string, error) {

  // Passe the YAML string through the template engine to resolve variables
  tmpl, err := template.New("tmpl").Delims("${{", "}}").Parse(srcString)
  if err != nil {
    return srcString, err
  }

  var agentURL string
  if viper.GetString("server.agent_url") != "" {
    agentURL = viper.GetString("server.agent_url")
  } else {
    agentURL = viper.GetString("server.url")
  }

  // Get the wildcard domain without the *
  wildcardDomain := viper.GetString("server.wildcard_domain");
  if wildcardDomain != "" && wildcardDomain[0] == '*' {
    wildcardDomain = wildcardDomain[1:]
  }

  data := map[string]interface{}{
    "space": map[string]interface{}{
      "id":   "",
      "name": "",
    },
    "template": map[string]interface{}{
      "id": "",
      "name": "",
    },
    "user": map[string]interface{}{
      "id": "",
      "username": "",
      "timezone": "",
      "email": "",
      "service_password": "",
    },
    "server": map[string]interface{}{
      "url": viper.GetString("server.url"),
      "agent_url": agentURL,
      "wildcard_domain": wildcardDomain,
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
      "id": t.Id,
      "name": t.Name,
    }
  }

  if user != nil {
    data["user"] = map[string]interface{}{
      "id": user.Id,
      "username": user.Username,
      "timezone": user.Timezone,
      "email": user.Email,
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
