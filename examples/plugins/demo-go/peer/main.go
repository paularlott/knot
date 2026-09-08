// Command demolib is the binary peer of the demo-go knot plugin — and, in
// the peer-manifest model, the WHOLE plugin: there is no companion main.py.
// It is a scriptling plugin-protocol server speaking JSON-RPC over stdio.
//
// knot spawns it from demo-go/bin/ at plugin load and handshakes it as
// "demolib". The handshake carries two things knot needs:
//
//   - the plugin's static declarations, as the [tool.knot] table under the
//     handshake's custom metadata (SetMetadata). knot reads and parses it
//     exactly like a pure-script plugin's metadata block — no plugin code
//     runs to produce declarations, so boot stays graceful and every node
//     computes the same result.
//   - the plugin's handler surface, as registered functions. Every handler
//     the manifest names (the page handler, its column handlers, the
//     [[tool.knot.handlers]] entries) is a function here, addressed by knot
//     as plugin.demolib.<fn>. Each receives the request dict knot passes to
//     every handler: {method, path, params, user}.
//
// This is the demonstration of a plugin whose logic lives entirely in a
// native peer: no scriptling in the plugin folder at all, just bin/ + assets/.
package main

import (
	"runtime"
	"time"

	"github.com/paularlott/scriptling/object"
	"github.com/paularlott/scriptling/plugin"
)

// manifest is the plugin's [tool.knot] declaration table, returned verbatim
// in the handshake's custom metadata. It is a constant — it never varies
// with runtime state — so every knot node parsing it agrees. knot is the
// only consumer that interprets "tool.knot"; scriptling just carries it.
//
// The shape mirrors what a pure-script plugin writes as TOML in its metadata
// block: keys and nested tables become maps and slices here.
func manifest() map[string]any {
	return map[string]any{
		"tool.knot": map[string]any{
			"version":     "1.0.0",
			"description": "Demo plugin whose logic lives entirely in a Go binary peer: the manifest and every handler are served by the peer, with no companion main.py. The /status dashboard charts the peer's own per-sample timing.",
			"permissions": []any{"view_status"},
			"logo_light":  "assets/logo-light.svg",
			"pages": []any{
				map[string]any{
					"path":       "/status",
					"handler":    "status_page",
					"label":      "Go Demo Status",
					"menu_label": "Go Demo",
					"permission": "view_status",
					"icon":       "assets/icon.svg",
				},
			},
			"handlers": []any{
				map[string]any{"handler": "peer_summary"},
				map[string]any{"handler": "peer_class"},
			},
		},
	}
}

// counter is the receiver behind the peer's Counter class: a stateful Go
// object scriptling constructs and calls methods on across the wire.
type counter struct {
	step int
	n    int
}

// info is the peer's build/runtime identity, surfaced by several handlers.
func info() map[string]any {
	return map[string]any{
		"peer": "demolib",
		"go":   runtime.Version(),
		"os":   runtime.GOOS,
		"arch": runtime.GOARCH,
	}
}

// samplesFromRequest reads the clamped sample count from the request params.
// Handlers receive the same request dict knot passes everywhere:
// {method, path, params, user}.
func samplesFromRequest(request map[string]any) int {
	samples := 20
	if params, ok := request["params"].(map[string]any); ok {
		switch v := params["samples"].(type) {
		case string:
			n := 0
			for _, r := range v {
				if r < '0' || r > '9' {
					n = -1
					break
				}
				n = n*10 + int(r-'0')
			}
			if n > 0 {
				samples = n
			}
		case float64:
			samples = int(v)
		case int:
			samples = v
		}
	}
	if samples < 5 {
		samples = 5
	}
	if samples > 100 {
		samples = 100
	}
	return samples
}

// latencies measures the peer doing real work: each sample times one
// greeting built the long way (byte by byte, forced to the heap) so the
// timer captures actual, varying nanoseconds rather than an elided no-op.
// The dashboard charts the peer's own per-sample timing.
func latencies(samples int) []float64 {
	out := make([]float64, 0, samples)
	for i := 0; i < samples; i++ {
		t0 := time.Now()
		sink = buildGreeting("knot") // real, measurable work (see sink)
		// Report microseconds: the per-sample work is sub-millisecond, so µs
		// keeps the chart and stats readable instead of rounding to zero.
		out = append(out, float64(time.Since(t0).Nanoseconds())/1e3)
	}
	return out
}

// sink defeats dead-code elimination so buildGreeting's work is actually
// performed and therefore measurable.
var sink string

// buildGreeting assembles the greeting one rune at a time — cheap but real
// work, enough for the per-sample timer to register non-zero, varying values.
func buildGreeting(name string) string {
	parts := []string{"hello ", name, ", from a Go peer inside knot"}
	b := make([]byte, 0, 48)
	for _, p := range parts {
		for j := 0; j < len(p); j++ {
			b = append(b, p[j])
		}
	}
	return string(b)
}

func stats(samples int) (lo, hi, avg float64) {
	vals := latencies(samples)
	if len(vals) == 0 {
		return 0, 0, 0
	}
	lo, hi = vals[0], vals[0]
	total := 0.0
	for _, v := range vals {
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
		total += v
	}
	return round3(lo), round3(hi), round3(total / float64(len(vals)))
}

func round3(v float64) float64 {
	return float64(int64(v*1000+0.5)) / 1000
}

func main() {
	server := plugin.NewServer("demolib", "1.0.0", "Demonstration Go binary peer for the knot demo-go plugin.")

	// The manifest: the plugin's static declarations, carried in the
	// handshake's custom metadata for knot to parse.
	server.SetMetadata(manifest())

	// --- Composable library surface (also usable by other plugins as
	//     plugin.demolib.greeting / .status / .Counter) ---

	server.RegisterFunc("status", object.NewFunctionBuilder().FunctionWithHelp(func() map[string]any {
		return info()
	}, "status() - report the peer's build and runtime (composable; also used by other plugins as plugin.demolib.status)."))

	server.RegisterFunc("greeting", object.NewFunctionBuilder().FunctionWithHelp(func(name string) string {
		return "hello " + name + ", from a Go peer inside knot"
	}, "greeting(name) - return a greeting from the peer."))

	server.RegisterClass(object.NewClassBuilder("Counter").
		Constructor(func(step int) *counter {
			return &counter{step: step}
		}).
		Method("next", func(self *counter) int {
			self.n += self.step
			return self.n
		}).
		Method("value", func(self *counter) int {
			return self.n
		}))

	// --- Handlers the manifest names (addressed by knot as
	//     plugin.demolib.<fn>). Each takes the request dict. ---

	// The page: returns the block-document layout. Its columns reference
	// the column handlers below by name.
	server.RegisterFunc("status_page", object.NewFunctionBuilder().FunctionWithHelp(func(request map[string]any) map[string]any {
		return map[string]any{
			"rows": []any{
				map[string]any{
					"title": "Peer demolib",
					"columns": []any{
						map[string]any{"id": "avg", "type": "stat", "handler": "stat_avg", "width": 1},
						map[string]any{"id": "lo", "type": "stat", "handler": "stat_lo", "width": 1},
						map[string]any{"id": "hi", "type": "stat", "handler": "stat_hi", "width": 1},
						map[string]any{"id": "samples", "type": "stat", "handler": "stat_samples", "width": 1},
					},
				},
				map[string]any{
					"columns": []any{
						map[string]any{"id": "latency", "type": "chart", "title": "Per-sample timing (µs)", "handler": "col_latency", "width": 3},
						map[string]any{"id": "form", "type": "form", "title": "Re-measure", "handler": "col_form", "width": 1},
					},
				},
				map[string]any{
					"columns": []any{
						map[string]any{"id": "peer", "type": "table", "title": "Peer", "handler": "col_peer", "width": 2},
						map[string]any{"id": "about", "type": "markdown", "title": "About", "handler": "col_about", "width": 2},
					},
				},
			},
		}
	}, "status_page(request) - the /status dashboard layout."))

	server.RegisterFunc("stat_avg", object.NewFunctionBuilder().Function(func(request map[string]any) map[string]any {
		_, _, avg := stats(samplesFromRequest(request))
		return map[string]any{"label": "Average", "value": avg, "unit": "µs"}
	}))
	server.RegisterFunc("stat_lo", object.NewFunctionBuilder().Function(func(request map[string]any) map[string]any {
		lo, _, _ := stats(samplesFromRequest(request))
		return map[string]any{"label": "Fastest", "value": lo, "unit": "µs"}
	}))
	server.RegisterFunc("stat_hi", object.NewFunctionBuilder().Function(func(request map[string]any) map[string]any {
		_, hi, _ := stats(samplesFromRequest(request))
		return map[string]any{"label": "Slowest", "value": hi, "unit": "µs"}
	}))
	server.RegisterFunc("stat_samples", object.NewFunctionBuilder().Function(func(request map[string]any) map[string]any {
		return map[string]any{"label": "Samples", "value": samplesFromRequest(request)}
	}))

	server.RegisterFunc("col_latency", object.NewFunctionBuilder().Function(func(request map[string]any) map[string]any {
		samples := samplesFromRequest(request)
		vals := latencies(samples)
		labels := make([]any, 0, samples)
		data := make([]any, 0, samples)
		for i, v := range vals {
			labels = append(labels, itoa(i+1))
			data = append(data, round3(v))
		}
		return map[string]any{
			"chart_type": "bar",
			"labels":     labels,
			"height":     220,
			"datasets": []any{
				map[string]any{"name": "Sample (µs)", "data": data, "color": "#f59e0b"},
			},
		}
	}))

	server.RegisterFunc("col_form", object.NewFunctionBuilder().Function(func(request map[string]any) map[string]any {
		if request["method"] == "POST" {
			return map[string]any{"status": "ok", "message": "Re-measured.", "refresh": true}
		}
		value := "20"
		if params, ok := request["params"].(map[string]any); ok {
			if s, ok := params["samples"].(string); ok && s != "" {
				value = s
			}
		}
		return map[string]any{
			"fields": []any{
				map[string]any{"type": "number", "name": "samples", "label": "Samples", "value": value},
			},
			"submit": "Re-measure",
		}
	}))

	server.RegisterFunc("col_peer", object.NewFunctionBuilder().Function(func(request map[string]any) map[string]any {
		i := info()
		return map[string]any{
			"columns": []any{
				map[string]any{"key": "prop", "label": "Property"},
				map[string]any{"key": "value", "label": "Value"},
			},
			"rows": []any{
				map[string]any{"prop": "peer", "value": i["peer"]},
				map[string]any{"prop": "go", "value": i["go"]},
				map[string]any{"prop": "os / arch", "value": stringOf(i["os"]) + " / " + stringOf(i["arch"])},
				map[string]any{"prop": "measured at", "value": time.Now().Format(time.RFC3339)},
			},
		}
	}))

	server.RegisterFunc("col_about", object.NewFunctionBuilder().Function(func(request map[string]any) map[string]any {
		return map[string]any{
			"markdown": "Each bar is one live measurement taken inside the Go peer (the time to build a greeting, sampled per bar). The peer is a subprocess spawned by knot at plugin load; it serves both the plugin's manifest (at handshake) and every handler this page calls.",
		}
	}))

	// [[tool.knot.handlers]] entries: addressable at the plugin root and
	// from other plugins' pages (cross-plugin composition). No declared
	// permission, so any logged-in user may call them.
	server.RegisterFunc("peer_summary", object.NewFunctionBuilder().Function(func(request map[string]any) map[string]any {
		i := info()
		return map[string]any{
			"summary": stringOf(i["peer"]) + " (" + stringOf(i["go"]) + ", " + stringOf(i["os"]) + "/" + stringOf(i["arch"]) + ")",
		}
	}))

	server.RegisterFunc("peer_class", object.NewFunctionBuilder().Function(func(request map[string]any) map[string]any {
		c := &counter{step: 3}
		first := func() int { c.n += c.step; return c.n }()
		second := func() int { c.n += c.step; return c.n }()
		return map[string]any{"first": first, "second": second, "value": c.n}
	}))

	if err := server.Run(); err != nil {
		panic(err)
	}
}

func stringOf(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
