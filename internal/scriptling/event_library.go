package scriptling

import (
	"context"

	"github.com/paularlott/scriptling/evaluator"
	"github.com/paularlott/scriptling/object"
)

const (
	EventParamsVarName = "__event_params"
	EventMetaVarName   = "__event"
)

func getEventParam(ctx context.Context, name string) object.Object {
	env := evaluator.GetEnvFromContext(ctx)
	if env == nil {
		return nil
	}

	paramsObj, ok := env.Get(EventParamsVarName)
	if !ok {
		return nil
	}

	paramsDict, ok := paramsObj.(*object.Dict)
	if !ok {
		return nil
	}

	pair, exists := paramsDict.GetByString(name)
	if !exists {
		return nil
	}

	return pair.Value
}

func getEventMeta(ctx context.Context, key string) object.Object {
	env := evaluator.GetEnvFromContext(ctx)
	if env == nil {
		return nil
	}

	metaObj, ok := env.Get(EventMetaVarName)
	if !ok {
		return nil
	}

	metaDict, ok := metaObj.(*object.Dict)
	if !ok {
		return nil
	}

	pair, exists := metaDict.GetByString(key)
	if !exists {
		return nil
	}

	return pair.Value
}

func GetEventLibrary() *object.Library {
	builder := object.NewLibraryBuilder("knot.event", "Knot event functions for sink scripts (server-side)")

	builder.FunctionWithHelp("get_string", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		if len(args) == 0 {
			return &object.Error{Message: "get_string() requires at least 1 argument (name)"}
		}
		name, errObj := args[0].AsString()
		if errObj != nil {
			return errObj
		}
		var defaultVal string
		if len(args) > 1 {
			defaultVal, errObj = args[1].AsString()
			if errObj != nil {
				return errObj
			}
		}
		val := getEventParam(ctx, name)
		if val == nil {
			return object.NewString(defaultVal)
		}
		s, errObj := val.AsString()
		if errObj != nil {
			return object.NewString(defaultVal)
		}
		return object.NewString(s)
	}, "get_string(name, default='') - Get a payload parameter as string")

	builder.FunctionWithHelp("get_int", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		if len(args) == 0 {
			return &object.Error{Message: "get_int() requires at least 1 argument (name)"}
		}
		name, errObj := args[0].AsString()
		if errObj != nil {
			return errObj
		}
		var defaultVal int64
		if len(args) > 1 {
			defaultVal, errObj = args[1].CoerceInt()
			if errObj != nil {
				return errObj
			}
		}
		val := getEventParam(ctx, name)
		if val == nil {
			return object.NewInteger(defaultVal)
		}
		i, errObj := val.CoerceInt()
		if errObj != nil {
			return object.NewInteger(defaultVal)
		}
		return object.NewInteger(i)
	}, "get_int(name, default=0) - Get a payload parameter as integer")

	builder.FunctionWithHelp("get_bool", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		if len(args) == 0 {
			return &object.Error{Message: "get_bool() requires at least 1 argument (name)"}
		}
		name, errObj := args[0].AsString()
		if errObj != nil {
			return errObj
		}
		var defaultVal bool
		if len(args) > 1 {
			defaultVal, errObj = args[1].AsBool()
			if errObj != nil {
				return errObj
			}
		}
		val := getEventParam(ctx, name)
		if val == nil {
			return object.NewBoolean(defaultVal)
		}
		b, errObj := val.AsBool()
		if errObj != nil {
			return object.NewBoolean(defaultVal)
		}
		return object.NewBoolean(b)
	}, "get_bool(name, default=False) - Get a payload parameter as boolean")

	builder.FunctionWithHelp("get_list", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		if len(args) == 0 {
			return &object.Error{Message: "get_list() requires at least 1 argument (name)"}
		}
		name, errObj := args[0].AsString()
		if errObj != nil {
			return errObj
		}
		val := getEventParam(ctx, name)
		if val == nil {
			if len(args) > 1 {
				return args[1]
			}
			return &object.List{Elements: []object.Object{}}
		}
		if _, ok := val.(*object.List); ok {
			return val
		}
		return val
	}, "get_list(name, default=[]) - Get a payload parameter as list")

	builder.FunctionWithHelp("get_dict", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		if len(args) == 0 {
			return &object.Error{Message: "get_dict() requires at least 1 argument (name)"}
		}
		name, errObj := args[0].AsString()
		if errObj != nil {
			return errObj
		}
		val := getEventParam(ctx, name)
		if val == nil {
			if len(args) > 1 {
				return args[1]
			}
			return object.NewStringDict(map[string]object.Object{})
		}
		if _, ok := val.(*object.Dict); ok {
			return val
		}
		return val
	}, "get_dict(name, default={}) - Get a payload parameter as dict")

	builder.FunctionWithHelp("type", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		val := getEventMeta(ctx, "type")
		if val == nil {
			return object.NewString("")
		}
		return val
	}, "type() - Get the event type string")

	builder.FunctionWithHelp("id", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		val := getEventMeta(ctx, "id")
		if val == nil {
			return object.NewString("")
		}
		return val
	}, "id() - Get the event UUIDv7 id")

	builder.FunctionWithHelp("ts", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		val := getEventMeta(ctx, "ts")
		if val == nil {
			return object.NewString("")
		}
		return val
	}, "ts() - Get the event HLC timestamp string")

	builder.FunctionWithHelp("space", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		val := getEventMeta(ctx, "space")
		if val == nil {
			return object.NewStringDict(map[string]object.Object{})
		}
		return val
	}, "space() - Get the source space dict")

	builder.FunctionWithHelp("space_urls", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		val := getEventMeta(ctx, "space_urls")
		if val == nil {
			return object.NewStringDict(map[string]object.Object{})
		}
		return val
	}, "space_urls() - Get the source space URLs dict")

	builder.FunctionWithHelp("actor", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		val := getEventMeta(ctx, "actor")
		if val == nil {
			return object.NewStringDict(map[string]object.Object{})
		}
		return val
	}, "actor() - Get the actor dict (id, username, kind)")

	builder.FunctionWithHelp("custom", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		val := getEventMeta(ctx, "custom")
		if val == nil {
			return object.NewStringDict(map[string]object.Object{})
		}
		return val
	}, "custom() - Get custom fields dict")

	return builder.Build()
}
