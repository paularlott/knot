// Command demolib is the binary peer of the demo-go knot plugin: a
// scriptling plugin-protocol server speaking JSON-RPC over stdio. knot
// spawns it from demo-go/bin/ at plugin load, handshakes it as "demolib",
// and the plugin's scripts reach it as plugin.demolib.
package main

import (
	"runtime"

	"github.com/paularlott/scriptling/object"
	"github.com/paularlott/scriptling/plugin"
)

func main() {
	server := plugin.NewServer("demolib", "1.0.0", "Demonstration Go binary peer for the knot demo-go plugin.")

	server.RegisterFunc("status", object.NewFunctionBuilder().FunctionWithHelp(func() map[string]any {
		return map[string]any{
			"peer":    "demolib",
			"go":      runtime.Version(),
			"os":      runtime.GOOS,
			"arch":    runtime.GOARCH,
		}
	}, "status() - report the peer's build and runtime."))

	server.RegisterFunc("greeting", object.NewFunctionBuilder().FunctionWithHelp(func(name string) string {
		return "hello " + name + ", from a Go peer inside knot"
	}, "greeting(name) - return a greeting from the peer."))

	if err := server.Run(); err != nil {
		panic(err)
	}
}
