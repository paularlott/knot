package api

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/plugins"
	"github.com/paularlott/knot/internal/service"
)

// maxOptionsInErrors caps how many option keys an error message lists; the
// remainder is summarised by count so the message stays readable (and cheap
// for an LLM's context) on long lists.
const maxOptionsInErrors = 20

// formatOptionList renders option keys for an error message.
func formatOptionList(keys []string) string {
	if len(keys) == 0 {
		return "no options"
	}
	if len(keys) <= maxOptionsInErrors {
		return strings.Join(keys, ", ")
	}
	return fmt.Sprintf("%s, … (%d more)", strings.Join(keys[:maxOptionsInErrors], ", "), len(keys)-maxOptionsInErrors)
}

// invalidCustomFieldOptions validates the values of a template's select and
// autocomplete custom fields against their option lists — the form restricts
// picking, the API for every caller. A value must be one of the field's
// option keys; a blank value is left to the required-field check (absent or
// empty passes here). It returns one segment per invalid field, formatted
// `name (got "value"; valid: a, b, …)` so the error alone tells the caller
// what to send.
//
// previous maps field names to their prior stored values; a field whose
// value is unchanged is not re-validated, so an update is never blocked by
// an option list that has moved on since the value was set (or by a plugin
// that is temporarily absent). Create passes nil.
//
// fetchKeys resolves a handler-backed field's option keys; manual option
// lists are checked without it. A fetch failure is returned as an error
// naming the field — the value cannot be verified, so the request is
// refused rather than silently accepted.
func invalidCustomFieldOptions(ctx context.Context, user *model.User, template *model.Template, values []model.SpaceCustomField, previous map[string]string, fetchKeys func(context.Context, *model.User, model.TemplateCustomField) ([]string, error)) ([]string, error) {
	if len(template.CustomFields) == 0 {
		return nil, nil
	}

	provided := make(map[string]string, len(values))
	for _, field := range values {
		provided[field.Name] = field.Value
	}

	var invalid []string
	for _, field := range template.CustomFields {
		if field.Type != "select" && field.Type != "autocomplete" {
			continue
		}

		value := provided[field.Name]
		if strings.TrimSpace(value) == "" {
			continue
		}
		if previous != nil && previous[field.Name] == value {
			continue
		}

		var keys []string
		switch {
		case len(field.Options) > 0:
			keys = field.Options
		case field.Handler != "":
			var err error
			keys, err = fetchKeys(ctx, user, field)
			if err != nil {
				return nil, fmt.Errorf("could not load options for custom field %q: %w", field.Name, err)
			}
		default:
			// No option source: template save rejects this shape, so
			// nothing to validate against.
			continue
		}

		if !slices.Contains(keys, value) {
			invalid = append(invalid, fmt.Sprintf("%s (got %q; valid: %s)", field.Name, value, formatOptionList(keys)))
		}
	}

	return invalid, nil
}

// describeMissingRequired enriches the field names returned by
// MissingRequiredCustomFields with each field's type and options, so an API
// error alone tells the caller — LLM or human — what to send. Enrichment
// never fails the request: a handler that cannot be reached degrades to
// naming it.
func describeMissingRequired(ctx context.Context, user *model.User, template *model.Template, missing []string, fetchKeys func(context.Context, *model.User, model.TemplateCustomField) ([]string, error)) []string {
	fields := make(map[string]model.TemplateCustomField, len(template.CustomFields))
	for _, field := range template.CustomFields {
		fields[field.Name] = field
	}

	out := make([]string, 0, len(missing))
	for _, name := range missing {
		field, ok := fields[name]
		if !ok {
			out = append(out, name)
			continue
		}

		fieldType := field.Type
		if fieldType == "" {
			fieldType = "text"
		}
		description := fmt.Sprintf("%s (%s", name, fieldType)
		switch {
		case len(field.Options) > 0:
			description += "; options: " + formatOptionList(field.Options)
		case field.Handler != "":
			if keys, err := fetchKeys(ctx, user, field); err == nil && len(keys) > 0 {
				description += "; options: " + formatOptionList(keys)
			} else {
				description += "; options from handler " + field.Handler
			}
		}
		out = append(out, description+")")
	}
	return out
}

// pluginFieldOptionKeys resolves a handler-backed custom field's option keys
// by dispatching the plugin field handler as the requesting user — the same
// dispatch, permission gate and timeout the options endpoint
// (/api/plugins/field-handlers/{id}) uses, so the two can never disagree
// about what is valid.
func pluginFieldOptionKeys(ctx context.Context, user *model.User, field model.TemplateCustomField) ([]string, error) {
	registry := plugins.GetRegistry()
	if registry == nil {
		return nil, fmt.Errorf("no plugins loaded")
	}
	plugin, handler := registry.FieldHandler(field.Handler)
	if plugin == nil || handler == nil {
		return nil, fmt.Errorf("field handler %q is not available", field.Handler)
	}
	if !user.PassesPluginGate(handler.Permission) {
		return nil, fmt.Errorf("field handler permission not granted")
	}

	timeout := 30 * time.Second
	if cfg := config.GetServerConfig(); cfg != nil && cfg.MCPToolTimeout > 0 {
		timeout = time.Duration(cfg.MCPToolTimeout) * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, err := service.DispatchPluginHandler(ctx, apiclient.NewMuxClient(user), user, plugin, handler.Handler, map[string]any{"_data": handler.Id})
	if err != nil {
		return nil, err
	}

	return optionKeysFromResult(result), nil
}

// optionKeysFromResult extracts the option keys from a field handler
// result: either a JSON array, or an object with an "options" array, whose
// entries are plain strings or {key, text} pairs — key first, text as
// fallback — mirroring the web UI's optKey.
func optionKeysFromResult(result any) []string {
	var list []any
	switch v := result.(type) {
	case []any:
		list = v
	case map[string]any:
		list, _ = v["options"].([]any)
	}

	keys := make([]string, 0, len(list))
	for _, entry := range list {
		switch e := entry.(type) {
		case string:
			keys = append(keys, e)
		case map[string]any:
			if key, ok := e["key"].(string); ok {
				keys = append(keys, key)
			} else if text, ok := e["text"].(string); ok {
				keys = append(keys, text)
			}
		}
	}
	return keys
}
