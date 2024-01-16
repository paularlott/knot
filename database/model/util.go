package model

import (
	"bytes"
	"text/template"

	"github.com/spf13/viper"
)

// Parse an input string and resolve knot variables
func ResolveVariables(srcString string, space *Space, user *User, variables *map[string]interface{}) (string, error) {

  // Passe the YAML string through the template engine to resolve variables
  tmpl, err := template.New("tmpl").Delims("${{", "}}").Parse(srcString)
  if err != nil {
    return srcString, err
  }

  data := map[string]interface{}{
    "space": map[string]interface{}{
      "id": space.Id,
      "name": space.Name,
    },
    "user": map[string]interface{}{
      "id": user.Id,
      "username": user.Username,
    },
    "server": map[string]interface{}{
      "url": viper.GetString("server.url"),
    },
    "registry": map[string]interface{}{
      "address": viper.GetString("server.registry.address"),
      "username": viper.GetString("server.registry.username"),
      "password": viper.GetString("server.registry.password"),
    },
    "var": variables,
  }

  var tmplBytes bytes.Buffer
  err = tmpl.Execute(&tmplBytes, data)
  if err != nil {
      return srcString, err
  }

  return tmplBytes.String(), nil
}
