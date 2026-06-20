package scriptling

import (
	"context"
	"fmt"
	"sort"

	"github.com/paularlott/scriptling/conversion"
	"github.com/paularlott/scriptling/errors"
	"github.com/paularlott/scriptling/object"
)

// optionalMarker tags a property schema produced by optional() so the object()
// builder knows to keep it out of the JSON Schema "required" list. The marker
// is stripped before the schema is returned to callers or stored on a method.
const optionalMarker = "__knot_optional__"

// GetMethodsSchemaLibrary builds the knot.methods.schema library. Each function
// returns a JSON Schema fragment as a scriptling dict. Common constraints
// (description, default, enum) are accepted as keyword arguments alongside
// type-specific constraints (min_length, minimum, ...). Unknown kwargs are
// rejected. The extra={} kwarg is an escape hatch for less-common JSON Schema
// keywords; explicit kwargs win over keys in extra on conflict.
func GetMethodsSchemaLibrary() *object.Library {
	builder := object.NewLibraryBuilder("knot.methods.schema", "Build JSON Schema fragments for knot.methods Server params and results")

	builder.FunctionWithHelp("string", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return buildScalar("string", kwargs, map[string]string{
			"format":     "format",
			"min_length": "minLength",
			"max_length": "maxLength",
			"pattern":    "pattern",
		})
	}, `string(*, description="", default=None, enum=None, format=None, min_length=None, max_length=None, pattern=None, extra=None) - Build a string schema`)

	builder.FunctionWithHelp("integer", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return buildScalar("integer", kwargs, map[string]string{
			"minimum": "minimum",
			"maximum": "maximum",
		})
	}, `integer(*, description="", default=None, enum=None, minimum=None, maximum=None, extra=None) - Build an integer schema`)

	builder.FunctionWithHelp("number", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return buildScalar("number", kwargs, map[string]string{
			"minimum": "minimum",
			"maximum": "maximum",
		})
	}, `number(*, description="", default=None, enum=None, minimum=None, maximum=None, extra=None) - Build a number (float) schema`)

	builder.FunctionWithHelp("boolean", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return buildScalar("boolean", kwargs, nil)
	}, `boolean(*, description="", default=None, enum=None, extra=None) - Build a boolean schema`)

	builder.FunctionWithHelp("null", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return buildScalar("null", kwargs, nil)
	}, `null(*, description="", default=None, enum=None, extra=None) - Build a null schema`)

	builder.FunctionWithHelp("array", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		if err := errors.MinArgs(args, 1); err != nil {
			return err
		}
		items := conversion.ToGo(args[0])
		itemsMap, ok := items.(map[string]interface{})
		if !ok {
			return &object.Error{Message: "array() items must be a schema dict"}
		}
		schema := map[string]interface{}{
			"type":  "array",
			"items": itemsMap,
		}
		if err := applyConstraints(schema, kwargs, map[string]string{
			"min_items": "minItems",
			"max_items": "maxItems",
		}); err != nil {
			return &object.Error{Message: err.Error()}
		}
		return finalizeSchema(schema, kwargs)
	}, `array(items, *, description="", default=None, enum=None, min_items=None, max_items=None, extra=None) - Build an array schema. items is a schema dict from another builder.`)

	builder.FunctionWithHelp("object", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return buildObject(kwargs)
	}, `object(**properties, *, description="", default=None, enum=None, additional_properties=None, extra=None) - Build an object schema. Every non-listed kwarg becomes a property. Property schemas from optional() are excluded from required.`)

	builder.FunctionWithHelp("optional", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		if err := errors.MinArgs(args, 1); err != nil {
			return err
		}
		raw := conversion.ToGo(args[0])
		schema, ok := raw.(map[string]interface{})
		if !ok {
			return &object.Error{Message: "optional() requires a schema dict"}
		}
		// Reject unknown kwargs: only "default" is supported.
		for _, k := range kwargs.Keys() {
			if k != "default" {
				return &object.Error{Message: fmt.Sprintf("optional() unknown kwarg %q", k)}
			}
		}
		if defObj := kwargs.Get("default"); defObj != nil {
			schema["default"] = conversion.ToGo(defObj)
		}
		// Mark the schema so object() knows to skip it in required. The marker
		// is stripped by object() when assembling the final schema.
		schema[optionalMarker] = true
		return conversion.FromGo(schema)
	}, `optional(schema, *, default=None) - Mark an object property as optional. Optionally sets a default value.`)

	return builder.Build()
}

// commonConstraints are the kwargs accepted by every scalar builder.
var commonConstraints = map[string]string{
	"description": "description",
	"default":     "default",
	"enum":        "enum",
}

// buildScalar builds a {"type": <type>, ...} schema from common constraints
// plus the type-specific constraints in extra.
func buildScalar(typ string, kwargs object.Kwargs, typeSpecific map[string]string) object.Object {
	schema := map[string]interface{}{"type": typ}
	if err := applyConstraints(schema, kwargs, typeSpecific); err != nil {
		return &object.Error{Message: err.Error()}
	}
	return finalizeSchema(schema, kwargs)
}

// buildObject assembles an object schema. Every kwarg that is not in the
// reserved set (description, default, enum, additional_properties, extra) is
// treated as a property name; its value must be a schema dict from another
// builder. Property schemas produced by optional() are kept out of required.
func buildObject(kwargs object.Kwargs) object.Object {
	reserved := map[string]bool{
		"description":           true,
		"default":               true,
		"enum":                  true,
		"additional_properties": true,
		"extra":                 true,
	}

	schema := map[string]interface{}{"type": "object"}
	properties := map[string]interface{}{}
	var required []string
	additionalPropsSet := false

	for _, key := range kwargs.Keys() {
		val := kwargs.Get(key)
		if val == nil {
			continue
		}
		if reserved[key] {
			// Reserved kwargs other than additional_properties are applied
			// below via applyObjectScalars.
			if key == "additional_properties" {
				schema["additionalProperties"] = conversion.ToGo(val)
				additionalPropsSet = true
			}
			continue
		}
		// Property name.
		propSchema, err := asPropertySchema(key, conversion.ToGo(val))
		if err != nil {
			return &object.Error{Message: err.Error()}
		}
		properties[key] = propSchema
		if _, optional := propSchema[optionalMarker]; !optional {
			required = append(required, key)
		}
		delete(propSchema, optionalMarker)
	}

	schema["properties"] = properties
	if len(required) > 0 {
		sort.Strings(required)
		schema["required"] = required
	}
	if !additionalPropsSet {
		// Knot's docs/MCP tooling default to disallowing extras. JSON Schema's
		// own default is true; we choose false to keep method params tight.
		schema["additionalProperties"] = false
	}

	// Pull out description/default/enum directly. Unlike scalar builders,
	// object() does not reject unknown kwargs because property names are
	// arbitrary strings.
	if v := kwargs.Get("description"); v != nil {
		schema["description"] = conversion.ToGo(v)
	}
	if v := kwargs.Get("default"); v != nil {
		schema["default"] = conversion.ToGo(v)
	}
	if v := kwargs.Get("enum"); v != nil {
		schema["enum"] = conversion.ToGo(v)
	}

	return finalizeSchema(schema, kwargs)
}

func asPropertySchema(name string, value interface{}) (map[string]interface{}, error) {
	m, ok := value.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("property %q must be a schema dict from a knot.methods.schema builder", name)
	}
	return m, nil
}

// applyConstraints copies allowed kwargs into the schema under their JSON
// Schema key names. typeSpecific maps scriptling kwarg name -> JSON Schema
// key. Common constraints (description, default, enum) are always allowed.
// Returns an error if an unknown kwarg (not common, not typeSpecific, not
// "extra") is seen.
func applyConstraints(schema map[string]interface{}, kwargs object.Kwargs, typeSpecific map[string]string) error {
	allowed := map[string]string{}
	for k, v := range commonConstraints {
		allowed[k] = v
	}
	for k, v := range typeSpecific {
		allowed[k] = v
	}

	for _, key := range kwargs.Keys() {
		if key == "extra" {
			continue
		}
		jsonKey, ok := allowed[key]
		if !ok {
			return fmt.Errorf("unknown kwarg %q", key)
		}
		val := kwargs.Get(key)
		if val == nil {
			continue
		}
		schema[jsonKey] = conversion.ToGo(val)
	}
	return nil
}

// finalizeSchema merges the extra={} escape hatch into the schema. Explicit
// kwargs already on the schema win over keys in extra (documented precedence).
func finalizeSchema(schema map[string]interface{}, kwargs object.Kwargs) object.Object {
	if extraObj := kwargs.Get("extra"); extraObj != nil {
		extra := conversion.ToGo(extraObj)
		extraMap, ok := extra.(map[string]interface{})
		if !ok {
			return &object.Error{Message: "extra must be a dict of JSON Schema keywords"}
		}
		for k, v := range extraMap {
			if _, present := schema[k]; !present {
				schema[k] = v
			}
		}
	}
	return conversion.FromGo(schema)
}
