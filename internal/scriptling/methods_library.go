package scriptling

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/internal/methods"
	"github.com/paularlott/scriptling"
	"github.com/paularlott/scriptling/conversion"
	"github.com/paularlott/scriptling/errors"
	"github.com/paularlott/scriptling/object"
)

// serverState is the typed receiver for the Server class. It collects the
// in-progress Registration between calls to method() and register().
type serverState struct {
	reg methods.Registration
}

// methodsRegistrar is the agent-side hook that publishes a Registration to the
// Knot server. It is set by the agent before any user script runs. It is nil
// in environments where method registration is not available.
var methodsRegistrar func(*methods.Registration) error

// methodsUnregisterAll is the agent-side hook that removes ALL methods for the
// current space, stops the method server, and clears the stashed registration.
// nil in environments where method registration is not available.
var methodsUnregisterAll func() error

// SetMethodsRegistrar installs the agent-side registrar. Called by the agent
// (or by knot run-script) before evaluating user scripts that use knot.methods.
func SetMethodsRegistrar(registrar func(*methods.Registration) error) {
	methodsRegistrar = registrar
}

func SetMethodsUnregisterAll(fn func() error) {
	methodsUnregisterAll = fn
}

// RegisterMethodLibraries registers knot.methods and knot.methods.schema on a
// scriptling env. This is opt-in: daemon-side script execution contexts
// (startup scripts via CmdExecuteScript, `knot methods register file.py`)
// call this so scripts running in the agent can register methods. CLI-side
// contexts (e.g. `knot run-script`) do not call it — those scripts cannot
// even import knot.methods, which makes the agent/CLI boundary explicit.
// Registering the libraries is independent of methodsRegistrar: the libraries
// give scripts a way to call Server(...).register(), and methodsRegistrar
// (installed once at daemon startup) is what register() actually invokes.
func RegisterMethodLibraries(env *scriptling.Scriptling) {
	env.RegisterLibrary(GetMethodsLibrary())
	env.RegisterLibrary(GetMethodsSchemaLibrary())
}

// GetMethodsLibrary returns the knot.methods library, which exposes the
// Server class. The schema submodule is attached as a Dict constant so that
// `from knot.methods import schema` and `knot.methods.schema` both resolve
// to the same set of builder functions (mirroring the pattern scriptling's
// own runtime library uses for submodules like runtime.kv).
func GetMethodsLibrary() *object.Library {
	schemaLib := GetMethodsSchemaLibrary()
	builder := object.NewLibraryBuilder("knot.methods", "Register JSON-RPC space methods from a running space")
	builder.Constant("Server", buildServerClass())
	builder.Constant("schema", schemaLib.GetDict())
	return builder.Build()
}

func buildServerClass() *object.Class {
	cb := object.NewClassBuilder("Server")

	// Constructor: Server(command, *, type="stdio", timeout=30, args=None, mode="concurrent")
	// `type` defaults to "stdio" (the only currently supported transport) and
	// is exposed as a kwarg so future transports can be opted into without
	// forcing every caller to spell out the default.
	cb.Constructor(func(ctx context.Context, kwargs object.Kwargs, command string) (*serverState, error) {
		state := &serverState{}
		state.reg.Server.Type = methods.ServerTypeStdio
		state.reg.Server.Command = command
		state.reg.Server.Timeout = 30
		state.reg.Server.Mode = methods.ModeConcurrent

		if v := kwargs.Get("type"); v != nil {
			t, errObj := v.AsString()
			if errObj != nil {
				return nil, conversion.ToGoError(errObj)
			}
			state.reg.Server.Type = t
		}
		if v := kwargs.Get("timeout"); v != nil {
			t, errObj := v.AsInt()
			if errObj != nil {
				return nil, conversion.ToGoError(errObj)
			}
			state.reg.Server.Timeout = int(t)
		}
		if v := kwargs.Get("mode"); v != nil {
			mode, errObj := v.AsString()
			if errObj != nil {
				return nil, conversion.ToGoError(errObj)
			}
			state.reg.Server.Mode = mode
		}
		if v := kwargs.Get("args"); v != nil {
			state.reg.Server.Args = toStringSlice(v)
		}
		for _, k := range kwargs.Keys() {
			switch k {
			case "type", "timeout", "mode", "args":
			default:
				return nil, fmt.Errorf("Server() unknown kwarg %q", k)
			}
		}
		return state, nil
	})

	// server.method(*, name, description, local_name="", keywords=[], scope="private",
	//                groups=[], mcp_tool=False, params=None, result=None)
	// name may be passed positionally or as a keyword argument.
	cb.MethodWithHelp("method", func(self *serverState, ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		var name string
		if len(args) >= 1 {
			s, err := args[0].AsString()
			if err != nil {
				return errors.ParameterError("name", err)
			}
			name = s
		} else if v := kwargs.Get("name"); v != nil {
			s, err := v.AsString()
			if err != nil {
				return errors.ParameterError("name", err)
			}
			name = s
		} else {
			return &object.Error{Message: "method() requires a name"}
		}

		def := methods.MethodDefinition{
			Name:  name,
			Scope: methods.ScopePrivate,
		}

		if v := kwargs.Get("local_name"); v != nil {
			s, errObj := v.AsString()
			if errObj != nil {
				return errors.ParameterError("local_name", errObj)
			}
			def.LocalName = s
		}
		if v := kwargs.Get("description"); v != nil {
			s, errObj := v.AsString()
			if errObj != nil {
				return errors.ParameterError("description", errObj)
			}
			def.Description = s
		}
		if v := kwargs.Get("scope"); v != nil {
			s, errObj := v.AsString()
			if errObj != nil {
				return errors.ParameterError("scope", errObj)
			}
			def.Scope = s
		}
		if v := kwargs.Get("keywords"); v != nil {
			def.Keywords = toStringSlice(v)
		}
		if v := kwargs.Get("groups"); v != nil {
			def.Groups = toStringSlice(v)
		}
		if v := kwargs.Get("mcp_tool"); v != nil {
			b, errObj := v.AsBool()
			if errObj != nil {
				return errors.ParameterError("mcp_tool", errObj)
			}
			def.MCPTool = b
		}
		if v := kwargs.Get("params"); v != nil {
			def.ParamsSchema = toSchemaMap("params", v)
			if def.ParamsSchema == nil {
				return &object.Error{Message: "method() params must be a schema dict"}
			}
		}
		if v := kwargs.Get("result"); v != nil {
			def.ResultSchema = toSchemaMap("result", v)
			if def.ResultSchema == nil {
				return &object.Error{Message: "method() result must be a schema dict"}
			}
		}
		if v := kwargs.Get("events"); v != nil {
			def.Events = toStringSlice(v)
		}
		if v := kwargs.Get("event_sinks"); v != nil {
			def.EventSinks = toStringSlice(v)
		}
		for _, k := range kwargs.Keys() {
			switch k {
			case "name", "local_name", "description", "scope", "keywords", "groups", "mcp_tool", "params", "result", "events", "event_sinks":
			default:
				return &object.Error{Message: fmt.Sprintf("method() unknown kwarg %q", k)}
			}
		}

		self.reg.Methods = append(self.reg.Methods, def)
		return object.NewBoolean(true)
	}, `method(name, *, local_name="", description="", scope="private", keywords=[], groups=[], mcp_tool=False, params=None, result=None, events=[], event_sinks=[]) - Add a method definition`)

	// server.register() - validate and publish the collected registration.
	cb.MethodWithHelp("register", func(self *serverState, ctx context.Context) object.Object {
		if methodsRegistrar == nil {
			return &object.Error{Message: "method registration is not available in this environment"}
		}
		if err := methodsRegistrar(&self.reg); err != nil {
			return &object.Error{Message: err.Error()}
		}
		return object.NewBoolean(true)
	}, `register() - Validate and publish the current method registration`)

	// server.unregister(name=None) - remove all methods, or one method by name.
	// With no args: calls methodsUnregisterAll (stops the method server, clears
	// everything). With a name: removes that method from the local registration
	// and re-publishes the reduced set via methodsRegistrar.
	cb.MethodWithHelp("unregister", func(self *serverState, ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		// No args — unregister all.
		if len(args) == 0 {
			if methodsUnregisterAll == nil {
				return &object.Error{Message: "method registration is not available in this environment"}
			}
			if err := methodsUnregisterAll(); err != nil {
				return &object.Error{Message: err.Error()}
			}
			return object.NewBoolean(true)
		}

		// One arg — unregister a specific method by name.
		if methodsRegistrar == nil {
			return &object.Error{Message: "method registration is not available in this environment"}
		}
		name, errObj := args[0].AsString()
		if errObj != nil {
			return errors.ParameterError("name", errObj)
		}

		found := false
		filtered := self.reg.Methods[:0]
		for _, m := range self.reg.Methods {
			if m.Name == name {
				found = true
				continue
			}
			filtered = append(filtered, m)
		}
		if !found {
			return &object.Error{Message: fmt.Sprintf("method %q is not registered", name)}
		}
		self.reg.Methods = filtered

		if len(self.reg.Methods) == 0 {
			// Removing the last method — use the full unregister path.
			if methodsUnregisterAll == nil {
				return &object.Error{Message: "method registration is not available in this environment"}
			}
			if err := methodsUnregisterAll(); err != nil {
				return &object.Error{Message: err.Error()}
			}
			return object.NewBoolean(true)
		}

		// Re-publish the reduced registration.
		if err := methodsRegistrar(&self.reg); err != nil {
			return &object.Error{Message: err.Error()}
		}
		return object.NewBoolean(true)
	}, `unregister(name=None) - Remove all methods, or one method by name`)

	return cb.Build()
}

// toSchemaMap converts a scriptling value to a JSON Schema map. The value can
// be a dict from knot.methods.schema builders, or a raw dict that already
// contains JSON Schema keywords.
func toSchemaMap(field string, obj object.Object) map[string]any {
	v := conversion.ToGo(obj)
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil
	}
	return map[string]any(m)
}

// toStringSlice converts a scriptling list/tuple of strings to []string.
// Non-string elements are skipped silently to keep the API permissive.
func toStringSlice(obj object.Object) []string {
	if obj == nil {
		return nil
	}
	items, ok := conversion.ToGo(obj).([]interface{})
	if !ok {
		// Allow a single string to be treated as a one-element list.
		if s, ok := conversion.ToGo(obj).(string); ok {
			return []string{s}
		}
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
