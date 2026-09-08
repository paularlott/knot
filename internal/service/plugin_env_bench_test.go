package service

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/plugins"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/scriptling"
)

// benchFixture loads a small plugin shaped like a realistic entry (a dozen
// handlers, module helpers) for cost measurement.
func benchFixture(b *testing.B) *plugins.Plugin {
	b.Helper()
	rest.SetAPIMux(http.NewServeMux())
	config.SetServerConfig(&config.ServerConfig{})

	dir := b.TempDir()
	pluginDir := filepath.Join(dir, "bench")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		b.Fatal(err)
	}
	source := `# /// script
# requires-scriptling = ">=0.1"
#
# [tool.knot]
# version = "1.0"
# ///
def helper_a(x):
    return x * 2


def handler_0(request):
    return {"v": helper_a(1)}


def handler_1(request):
    return {"v": helper_a(2)}


def handler_2(request):
    return {"v": helper_a(3)}


def handler_3(request):
    return {"v": helper_a(4)}


def handler_4(request):
    return {"v": helper_a(5)}


def handler_5(request):
    return {"v": helper_a(6)}


def handler_6(request):
    return {"v": helper_a(7)}


def handler_7(request):
    return {"v": helper_a(8)}
`
	if err := os.WriteFile(filepath.Join(pluginDir, "main.py"), []byte(source), 0o644); err != nil {
		b.Fatal(err)
	}
	registry, err := plugins.Load(dir)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		registry.Close()
		plugins.SetRegistry(nil)
	})
	return registry.ByName("bench")
}

// BenchmarkPluginEnvCold measures the full cold path: env construction
// (~40 library registrations, loader chain) plus entry eval.
func BenchmarkPluginEnvCold(b *testing.B) {
	plugin := benchFixture(b)
	user := &model.User{Username: "bench", Id: "u-bench"}
	ctx := context.Background()
	client := apiclient.NewMuxClient(user)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		env, err := NewPluginScriptlingEnv(client, user, plugin)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := env.EvalWithContext(ctx, plugin.EntrySource); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkPluginDispatchLease measures a full warm lease: acquire (Reset,
// identity rebind, entry re-eval), dispatch, release — the per-request cost
// once the plugin's env is pooled.
func BenchmarkPluginDispatchLease(b *testing.B) {
	plugin := benchFixture(b)
	user := &model.User{Username: "bench", Id: "u-bench"}
	ctx := context.Background()
	client := apiclient.NewMuxClient(user)

	// Warm the pool.
	env, err := AcquirePluginEnv(ctx, client, user, plugin)
	if err != nil {
		b.Fatal(err)
	}
	ReleasePluginEnv(env, plugin)

	qualified := QualifiedHandler(plugin, "handler_0")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		env, err := AcquirePluginEnv(ctx, client, user, plugin)
		if err != nil {
			b.Fatal(err)
		}
		request := RequestObject("GET", "/plugins/bench/handler_0", map[string]any{}, user)
		if _, err := env.CallFunctionWithContext(ctx, qualified, request); err != nil {
			b.Fatal(err)
		}
		ReleasePluginEnv(env, plugin)
	}
}

// BenchmarkPluginEnvMemory measures retained memory per live env: builds
// twenty (kept alive) and reports the heap delta per env.
func BenchmarkPluginEnvMemory(b *testing.B) {
	plugin := benchFixture(b)
	user := &model.User{Username: "bench", Id: "u-bench"}
	ctx := context.Background()
	client := apiclient.NewMuxClient(user)

	var before, after runtime.MemStats

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		envs := make([]*scriptling.Scriptling, 0, 20)
		runtime.GC()
		runtime.ReadMemStats(&before)
		for j := 0; j < 20; j++ {
			env, err := NewPluginScriptlingEnv(client, user, plugin)
			if err != nil {
				b.Fatal(err)
			}
			if _, err := env.EvalWithContext(ctx, plugin.EntrySource); err != nil {
				b.Fatal(err)
			}
			envs = append(envs, env)
		}
		runtime.ReadMemStats(&after)
		b.ReportMetric(float64(after.TotalAlloc-before.TotalAlloc)/20/1024, "env-KB")
	}
}
