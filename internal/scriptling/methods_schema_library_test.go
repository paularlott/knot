package scriptling

import (
	"context"
	"strings"
	"testing"

	"github.com/paularlott/scriptling"
	"github.com/paularlott/scriptling/object"
)

// evalSchema runs a snippet of Scriptling that imports knot.methods.schema
// under the alias `s` and leaves the final value in `result`.
func evalSchema(t *testing.T, script string) interface{} {
	t.Helper()
	env := scriptling.New()
	env.RegisterLibrary(GetMethodsSchemaLibrary())
	env.RegisterLibrary(GetMethodsLibrary())
	full := "import knot.methods.schema as s\nresult = None\n" + script
	res, err := env.EvalWithContext(context.Background(), full)
	if err != nil {
		t.Fatalf("eval error: %v\nscript:\n%s", err, full)
	}
	if obj, ok := res.(*object.Error); ok {
		t.Fatalf("script returned error: %s\nscript:\n%s", obj.Message, full)
	}
	val, objErr := env.GetVar("result")
	if objErr != nil {
		t.Fatalf("GetVar(result) error: %v", objErr)
	}
	return val
}

func evalSchemaErr(t *testing.T, script string) string {
	t.Helper()
	env := scriptling.New()
	env.RegisterLibrary(GetMethodsSchemaLibrary())
	full := "import knot.methods.schema as s\n" + script
	res, _ := env.EvalWithContext(context.Background(), full)
	if obj, ok := res.(*object.Error); ok {
		return obj.Message
	}
	t.Fatalf("expected error, got %#v", res)
	return ""
}

func TestSchemaStringBuildsConstraints(t *testing.T) {
	got := evalSchema(t, `result = s.string(description="A query", format="date-time", min_length=1, max_length=100, pattern="^[a-z]+$")
`)
	m, ok := got.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", got)
	}
	if m["type"] != "string" {
		t.Errorf("type: got %v", m["type"])
	}
	if m["description"] != "A query" {
		t.Errorf("description: got %v", m["description"])
	}
	if m["format"] != "date-time" {
		t.Errorf("format: got %v", m["format"])
	}
	if m["minLength"] != int64(1) {
		t.Errorf("minLength: got %v", m["minLength"])
	}
	if m["maxLength"] != int64(100) {
		t.Errorf("maxLength: got %v", m["maxLength"])
	}
	if m["pattern"] != "^[a-z]+$" {
		t.Errorf("pattern: got %v", m["pattern"])
	}
}

func TestSchemaIntegerExtraMergePrecedence(t *testing.T) {
	got := evalSchema(t, `result = s.integer(minimum=5, extra={"minimum": 0, "format": "int32"})
`)
	m := got.(map[string]interface{})
	if m["minimum"] != int64(5) {
		t.Errorf("explicit kwarg should win, got %v", m["minimum"])
	}
	if m["format"] != "int32" {
		t.Errorf("extra keyword should merge, got %v", m["format"])
	}
}

func TestSchemaRejectsUnknownKwarg(t *testing.T) {
	msg := evalSchemaErr(t, `s.string(banana="no")
`)
	if !strings.Contains(msg, "banana") {
		t.Fatalf("error should mention unknown kwarg, got: %s", msg)
	}
}

func TestSchemaObjectRequiredAndOptional(t *testing.T) {
	got := evalSchema(t, `result = s.object(
    query=s.string(),
    limit=s.optional(s.integer(), default=10),
)
`)
	m := got.(map[string]interface{})
	if m["type"] != "object" {
		t.Fatalf("type: got %v", m["type"])
	}
	props := m["properties"].(map[string]interface{})
	if _, ok := props["query"]; !ok {
		t.Errorf("missing query property")
	}
	limitProp := props["limit"].(map[string]interface{})
	if _, hasMarker := limitProp[optionalMarker]; hasMarker {
		t.Errorf("optional marker should be stripped in final schema")
	}
	if limitProp["default"] != int64(10) {
		t.Errorf("default should be on property, got %v", limitProp["default"])
	}
	required, ok := m["required"].([]interface{})
	if !ok {
		t.Fatalf("required should be a list, got %T", m["required"])
	}
	if len(required) != 1 || required[0] != "query" {
		t.Errorf("required should be [query], got %v", required)
	}
	if m["additionalProperties"] != false {
		t.Errorf("additionalProperties default should be false, got %v", m["additionalProperties"])
	}
}

func TestSchemaObjectAdditionalPropertiesOverride(t *testing.T) {
	got := evalSchema(t, `result = s.object(additional_properties=True)
`)
	m := got.(map[string]interface{})
	if m["additionalProperties"] != true {
		t.Errorf("expected override to true, got %v", m["additionalProperties"])
	}
}

func TestSchemaArray(t *testing.T) {
	got := evalSchema(t, `result = s.array(s.string(), min_items=1)
`)
	m := got.(map[string]interface{})
	if m["type"] != "array" {
		t.Fatalf("type: got %v", m["type"])
	}
	items := m["items"].(map[string]interface{})
	if items["type"] != "string" {
		t.Errorf("items.type: got %v", items["type"])
	}
	if m["minItems"] != int64(1) {
		t.Errorf("minItems: got %v", m["minItems"])
	}
}

func TestSchemaNestedObject(t *testing.T) {
	got := evalSchema(t, `result = s.object(
    filter=s.object(
        field=s.string(),
        value=s.string(),
    ),
)
`)
	m := got.(map[string]interface{})
	props := m["properties"].(map[string]interface{})
	filter := props["filter"].(map[string]interface{})
	if filter["type"] != "object" {
		t.Errorf("nested filter should be object, got %v", filter["type"])
	}
	if _, hasMarker := filter[optionalMarker]; hasMarker {
		t.Errorf("marker leaked into nested object")
	}
}
