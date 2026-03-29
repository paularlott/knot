package scriptling

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/scriptling/conversion"
	"github.com/paularlott/scriptling/errors"
	"github.com/paularlott/scriptling/object"
)

// GetApiClientLibrary returns the knot.apiclient Go transport library.
// In embedded contexts this replaces the Python apiclient.py transport stub.
// configure() and is_configured() are no-ops - the client is pre-configured.
// Tokens are never exposed to scripts.
func GetApiClientLibrary(client rest.RESTClient, userId string) *object.Library {
	builder := object.NewLibraryBuilder("knot.apiclient", "Knot API transport (embedded)")

	builder.FunctionWithHelp("configure", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return &object.Boolean{Value: true}
	}, "configure(url, token, insecure=False) - No-op in embedded mode; client is pre-configured")

	builder.FunctionWithHelp("is_configured", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return &object.Boolean{Value: true}
	}, "is_configured() - Always returns True in embedded mode")

	builder.FunctionWithHelp("get", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		if err := errors.MinArgs(args, 1); err != nil {
			return err
		}
		path, err := args[0].AsString()
		if err != nil {
			return errors.ParameterError("path", err)
		}

		// Append query params if provided
		if paramsObj := kwargs.Get("params"); paramsObj != nil {
			if paramsDict, ok := paramsObj.(*object.Dict); ok {
				params := url.Values{}
				for _, pair := range paramsDict.Pairs {
					key := pair.Key.Inspect()
					val := pair.Value.Inspect()
					params.Set(key, val)
				}
				if encoded := params.Encode(); encoded != "" {
					if strings.Contains(path, "?") {
						path += "&" + encoded
					} else {
						path += "?" + encoded
					}
				}
			}
		}

		var result interface{}
		statusCode, apiErr := client.GetJSON(ctx, path, &result)
		if apiErr != nil {
			return &object.Error{Message: fmt.Sprintf("API error: %v", apiErr)}
		}
		if statusCode >= 400 {
			return &object.Error{Message: fmt.Sprintf("API error: HTTP %d", statusCode)}
		}
		if result == nil {
			return &object.Null{}
		}
		return conversion.FromGo(result)
	}, "get(path, params=None) - Make a GET request, returns Dict or List")

	builder.FunctionWithHelp("post", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		if err := errors.MinArgs(args, 1); err != nil {
			return err
		}
		path, err := args[0].AsString()
		if err != nil {
			return errors.ParameterError("path", err)
		}

		var body interface{}
		if len(args) > 1 && args[1] != nil {
			if _, isNull := args[1].(*object.Null); !isNull {
				body = conversion.ToGo(args[1])
			}
		}

		var result interface{}
		statusCode, apiErr := client.PostJSON(ctx, path, body, &result, 200)
		if apiErr != nil {
			return &object.Error{Message: fmt.Sprintf("API error: %v", apiErr)}
		}
		if statusCode >= 400 {
			return &object.Error{Message: fmt.Sprintf("API error: HTTP %d", statusCode)}
		}
		if result == nil {
			return &object.Null{}
		}
		return conversion.FromGo(result)
	}, "post(path, body=None) - Make a POST request, returns Dict or List")

	builder.FunctionWithHelp("put", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		if err := errors.MinArgs(args, 1); err != nil {
			return err
		}
		path, err := args[0].AsString()
		if err != nil {
			return errors.ParameterError("path", err)
		}

		var body interface{}
		if len(args) > 1 && args[1] != nil {
			if _, isNull := args[1].(*object.Null); !isNull {
				body = conversion.ToGo(args[1])
			}
		}

		var result interface{}
		statusCode, apiErr := client.PutJSON(ctx, path, body, &result, 200)
		if apiErr != nil {
			return &object.Error{Message: fmt.Sprintf("API error: %v", apiErr)}
		}
		if statusCode >= 400 {
			return &object.Error{Message: fmt.Sprintf("API error: HTTP %d", statusCode)}
		}
		if result == nil {
			return &object.Null{}
		}
		return conversion.FromGo(result)
	}, "put(path, body=None) - Make a PUT request, returns Dict or List")

	builder.FunctionWithHelp("delete", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		if err := errors.MinArgs(args, 1); err != nil {
			return err
		}
		path, err := args[0].AsString()
		if err != nil {
			return errors.ParameterError("path", err)
		}

		var result interface{}
		statusCode, apiErr := client.Delete(ctx, path, nil, &result, 200)
		if apiErr != nil {
			return &object.Error{Message: fmt.Sprintf("API error: %v", apiErr)}
		}
		if statusCode >= 400 {
			return &object.Error{Message: fmt.Sprintf("API error: HTTP %d", statusCode)}
		}
		if result == nil {
			return &object.Null{}
		}
		return conversion.FromGo(result)
	}, "delete(path) - Make a DELETE request, returns Dict or List")

	return builder.Build()
}
