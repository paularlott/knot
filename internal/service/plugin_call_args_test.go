package service

import (
	"testing"

	"github.com/paularlott/scriptling/object"
)

// TestPluginCallArgs pins call(plugin, handler, params=None, method="GET")
// for both knot.plugin twins: params and method count whether given
// positionally or by keyword.
func TestPluginCallArgs(t *testing.T) {
	params := object.NewStringDict(map[string]object.Object{"word": object.NewString("hi")})
	kw := func(m map[string]object.Object) object.Kwargs { return object.NewKwargs(m) }
	str := object.NewString

	for _, tc := range []struct {
		name   string
		kwargs object.Kwargs
		args   []object.Object
		method string
		word   any
	}{
		{"positional params", kw(nil), []object.Object{str("p"), str("h"), params}, "GET", "hi"},
		{"keyword params", kw(map[string]object.Object{"params": params}), []object.Object{str("p"), str("h")}, "GET", "hi"},
		{"keyword params and method", kw(map[string]object.Object{"params": params, "method": str("post")}), []object.Object{str("p"), str("h")}, "POST", "hi"},
		{"positional method", kw(nil), []object.Object{str("p"), str("h"), params, str("POST")}, "POST", "hi"},
		{"None params", kw(map[string]object.Object{"params": &object.Null{}}), []object.Object{str("p"), str("h")}, "GET", nil},
	} {
		plugin, handler, got, method, errObj := pluginCallArgs(tc.kwargs, tc.args)
		if errObj != nil {
			t.Fatalf("%s: %v", tc.name, errObj.Inspect())
		}
		if plugin != "p" || handler != "h" || method != tc.method || got["word"] != tc.word {
			t.Errorf("%s: got %s %s %v %s", tc.name, plugin, handler, got, method)
		}
	}

	if _, _, _, _, errObj := pluginCallArgs(kw(map[string]object.Object{"param": params}), []object.Object{str("p"), str("h")}); errObj == nil {
		t.Error("misspelt keyword accepted silently")
	}
	if _, _, _, _, errObj := pluginCallArgs(kw(map[string]object.Object{"method": str("PUT")}), []object.Object{str("p"), str("h")}); errObj == nil {
		t.Error("method PUT accepted")
	}
}
