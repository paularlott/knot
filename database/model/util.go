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
    },
    "server": map[string]interface{}{
      "url": viper.GetString("server.url"),
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
    }
  }

  var tmplBytes bytes.Buffer
  err = tmpl.Execute(&tmplBytes, data)
  if err != nil {
      return srcString, err
  }

  return tmplBytes.String(), nil
}
