package model

import (
	"bytes"
	"text/template"
)

// Parse an input string and resolve knot variables
func ResolveVariables(srcString string, space *Space, user *User) (string, error) {

  // Passe the YAML string through the template engine to resolve variables
  tmpl, err := template.New("tmpl").Delims("$[", "]").Parse(srcString)
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
  }

  var tmplBytes bytes.Buffer
  err = tmpl.Execute(&tmplBytes, data)
  if err != nil {
      return srcString, err
  }

  return tmplBytes.String(), nil
}
