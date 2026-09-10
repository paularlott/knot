package service

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/plugins"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/scriptling/conversion"
)

// TestPluginConfigInjection drives the demo plugin's config_echo handler
// through the same dispatch path production uses (acquirePluginEnv +
// RequestObject): the plugin's configured table round-trips as
// request["config"], and an unconfigured plugin sees an empty table.
func TestPluginConfigInjection(t *testing.T) {
	rest.SetAPIMux(http.NewServeMux())
	config.SetServerConfig(&config.ServerConfig{})

	registry, err := plugins.LoadWithConfigs(filepath.Join("..", "..", "examples", "plugins"), map[string]map[string]any{
		"demo-scriptling": {"greeting": "hello from knot.toml"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer registry.Close()
	plugin := registry.ByName("demo-scriptling")
	if plugin == nil {
		t.Fatalf("demo plugin not loaded: %+v", registry.Failed())
	}

	user := &model.User{Id: "u-1", Username: "tester", Active: true}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	env, err := AcquirePluginEnv(ctx, apiclient.NewMuxClient(user), user, plugin)
	if err != nil {
		t.Fatalf("acquire env: %v", err)
	}
	defer ReleasePluginEnv(env, plugin)

	request := RequestObject("GET", "/plugins/demo-scriptling/config_echo", map[string]any{}, user, plugin.Config)
	result, err := env.CallFunctionWithContext(ctx, QualifiedHandler(plugin, "config_echo"), request)
	if err != nil {
		t.Fatalf("config_echo: %v", err)
	}
	got := conversion.ToGo(result)
	dict, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("result = %#v, want a dict", got)
	}
	if dict["greeting"] != "hello from knot.toml" {
		t.Fatalf("greeting = %v, config did not round-trip", dict["greeting"])
	}
	if dict["configured"] != true {
		t.Fatalf("configured = %v, want true", dict["configured"])
	}
	nested, ok := dict["config"].(map[string]any)
	if !ok || nested["greeting"] != "hello from knot.toml" {
		t.Fatalf("config echo = %#v", dict["config"])
	}

	// An unconfigured plugin sees a stable empty table, never a missing
	// key — plugin code can index request["config"] unconditionally.
	empty := RequestObject("GET", "/plugins/demo-scriptling/config_echo", map[string]any{}, user, nil)
	result, err = env.CallFunctionWithContext(ctx, QualifiedHandler(plugin, "config_echo"), empty)
	if err != nil {
		t.Fatalf("config_echo (unconfigured): %v", err)
	}
	got = conversion.ToGo(result)
	dict, ok = got.(map[string]any)
	if !ok || dict["configured"] != false || dict["greeting"] != "unconfigured" {
		t.Fatalf("unconfigured result = %#v, want defaults", got)
	}
}
