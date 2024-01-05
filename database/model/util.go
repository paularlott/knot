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
    "space_id": space.Id,
    "space_name": space.Name,
    "user_id": space.UserId,
    "username": user.Username,
  }

  var tmplBytes bytes.Buffer
  err = tmpl.Execute(&tmplBytes, data)
  if err != nil {
      return srcString, err
  }

  return tmplBytes.String(), nil
}
