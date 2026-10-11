package model

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"testing"
)

// legacyCustomField is TemplateCustomField as it was before layout flags:
// the hash of every existing template was computed over its %v.
type legacyCustomField struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Type        string   `json:"type,omitempty"`
	Handler     string   `json:"handler,omitempty"`
	Language    string   `json:"language,omitempty"`
	Default     string   `json:"default,omitempty"`
	Required    bool     `json:"required,omitempty"`
	Options     []string `json:"options,omitempty"`
}

func legacyHash(t *Template, fields []legacyCustomField) string {
	in := t.Job + t.Volumes + t.Platform + fmt.Sprintf("%t%t%t%t%t%t%v", t.WithTerminal, t.WithVSCodeTunnel, t.WithCodeServer, t.WithSSH, t.WithRunCommand, t.AllowNodeMigration, fields)
	sum := md5.Sum([]byte(in))
	return hex.EncodeToString(sum[:])
}

func TestTemplateHashUnchangedByShowOnCreate(t *testing.T) {
	for _, fields := range [][]TemplateCustomField{
		nil,
		{},
		{{Name: "branch", Description: "Branch", Default: "main"}},
		{{Name: "a", Type: "select", Options: []string{"x", "y"}, Required: true}, {Name: "b", Type: "bool"}},
	} {
		tpl := &Template{Job: "job", Volumes: "vols", Platform: "docker", WithTerminal: true, CustomFields: fields}
		var legacy []legacyCustomField
		if fields != nil {
			legacy = []legacyCustomField{}
			for _, f := range fields {
				legacy = append(legacy, legacyCustomField{f.Name, f.Description, f.Type, f.Handler, f.Language, f.Default, f.Required, f.Options})
			}
		}
		tpl.UpdateHash()
		if want := legacyHash(tpl, legacy); tpl.Hash != want {
			t.Fatalf("hash changed for existing template %v: got %s want %s", fields, tpl.Hash, want)
		}
		before := tpl.Hash
		for i := range tpl.CustomFields {
			tpl.CustomFields[i].ShowOnCreate = true
		}
		tpl.UpdateHash()
		if tpl.Hash != before {
			t.Fatalf("show_on_create changed the hash for %v", fields)
		}
	}
}
