package service

import (
	"os"
	"strings"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
)

// BuildStackVariableData builds the .stack.* template variable map for a space,
// exposing its sibling spaces within the same stack. Siblings are keyed by their
// stack-definition key (the space name with the stack prefix stripped), so a
// template can reference e.g. ${{ .stack.db.space.id }} or
// ${{ .stack.db.custom.password }}.
//
// Each sibling exposes the full variable set (space, template, user, custom, and
// the shared server/nomad/var groups). Returns nil when the space is not part of
// a stack or no siblings can be loaded, leaving .stack absent from the render
// data (references then resolve to empty, like any missing variable).
//
// This is injected into the model package via model.SetStackResolver at server
// startup to avoid the model <-> database import cycle.
func BuildStackVariableData(space *model.Space, variables map[string]interface{}) map[string]interface{} {
	if space == nil || space.Stack == "" {
		return nil
	}

	db := database.GetInstance()
	siblings, err := db.GetSpacesForUser(space.UserId)
	if err != nil || len(siblings) == 0 {
		return nil
	}

	cfg := config.GetServerConfig()

	// Shared (global) groups — identical for every space on this server/user.
	wildcardDomain := cfg.WildcardDomain
	if len(wildcardDomain) > 0 && wildcardDomain[0] == '*' {
		wildcardDomain = wildcardDomain[1:]
	}
	serverGroup := map[string]interface{}{
		"url":             strings.TrimSuffix(cfg.URL, "/"),
		"agent_endpoint":  cfg.AgentEndpoint,
		"wildcard_domain": wildcardDomain,
		"zone":            cfg.Zone,
		"timezone":        cfg.Timezone,
	}
	nomadGroup := map[string]interface{}{
		"dc":     os.Getenv("NOMAD_DC"),
		"region": os.Getenv("NOMAD_REGION"),
	}
	varGroup := variables
	if varGroup == nil {
		varGroup = map[string]interface{}{}
	}

	// Avoid repeated lookups for siblings that share a template or owner.
	templateCache := make(map[string]*model.Template)
	userCache := make(map[string]*model.User)

	stack := make(map[string]interface{})
	for _, sib := range siblings {
		if sib == nil || sib.IsDeleted || sib.Stack != space.Stack {
			continue
		}

		key := stackKey(sib)
		entry := buildSiblingEntry(sib, db, serverGroup, nomadGroup, varGroup, templateCache, userCache)
		stack[key] = entry

		// Also expose a dotted-safe alias with "-" -> "_". Go templates can't
		// dot-address a key containing "-", so the alias lets users write
		// ${{ .stack.space_1.custom.x }} while the literal key remains available
		// via index, e.g. ${{ (index .stack "space-1").custom.x }}. This is
		// collision-free because space names (and therefore stack keys) can
		// never contain "_" — they are URL-safe — so the alias can never shadow
		// a real sibling key.
		if alias := stackKeyAlias(key); alias != key {
			stack[alias] = entry
		}
	}

	if len(stack) == 0 {
		return nil
	}
	return stack
}

// stackKey returns the stack-definition key for a space: its name with the
// stack prefix (and trailing "-") removed, e.g. "myapp-db" -> "db".
func stackKey(s *model.Space) string {
	if s.StackPrefix != "" {
		return strings.TrimPrefix(s.Name, s.StackPrefix+"-")
	}
	return s.Name
}

// stackKeyAlias returns the dotted-safe form of a stack key, with every "-"
// replaced by "_". When the key contains no "-" the result equals the input.
// See BuildStackVariableData for the collision-freedom argument.
func stackKeyAlias(key string) string {
	return strings.ReplaceAll(key, "-", "_")
}

func buildSiblingEntry(
	sib *model.Space,
	db database.DbDriver,
	serverGroup, nomadGroup, varGroup map[string]interface{},
	templateCache map[string]*model.Template,
	userCache map[string]*model.User,
) map[string]interface{} {
	entry := map[string]interface{}{
		"space": map[string]interface{}{
			"id":           sib.Id,
			"name":         sib.Name,
			"stack":        sib.Stack,
			"stack_prefix": sib.StackPrefix,
			"first_boot":   sib.TemplateHash == "",
		},
		// Global groups shared across all spaces.
		"server": serverGroup,
		"nomad":  nomadGroup,
		"var":    varGroup,
	}

	// Custom fields for this sibling.
	custom := make(map[string]interface{})
	for _, f := range sib.CustomFields {
		custom[f.Name] = f.Value
	}
	entry["custom"] = custom

	// Template name/id for this sibling.
	if tmpl, ok := loadTemplate(sib.TemplateId, db, templateCache); ok {
		entry["template"] = map[string]interface{}{
			"id":   tmpl.Id,
			"name": tmpl.Name,
		}
	} else {
		entry["template"] = map[string]interface{}{"id": sib.TemplateId, "name": ""}
	}

	// Owner of this sibling (for stacks, normally the same user).
	if owner, ok := loadUser(sib.UserId, db, userCache); ok {
		entry["user"] = map[string]interface{}{
			"id":               owner.Id,
			"username":         owner.Username,
			"timezone":         owner.Timezone,
			"email":            owner.Email,
			"service_password": owner.ServicePassword,
		}
	} else {
		entry["user"] = map[string]interface{}{
			"id":               sib.UserId,
			"username":         "",
			"timezone":         "",
			"email":            "",
			"service_password": "",
		}
	}

	return entry
}

func loadTemplate(templateId string, db database.DbDriver, cache map[string]*model.Template) (*model.Template, bool) {
	if templateId == "" {
		return nil, false
	}
	if t, ok := cache[templateId]; ok {
		return t, t != nil
	}
	t, err := db.GetTemplate(templateId)
	if err != nil || t == nil || t.IsDeleted {
		cache[templateId] = nil
		return nil, false
	}
	cache[templateId] = t
	return t, true
}

func loadUser(userId string, db database.DbDriver, cache map[string]*model.User) (*model.User, bool) {
	if userId == "" {
		return nil, false
	}
	if u, ok := cache[userId]; ok {
		return u, u != nil
	}
	u, err := db.GetUser(userId)
	if err != nil || u == nil {
		cache[userId] = nil
		return nil, false
	}
	cache[userId] = u
	return u, true
}
