package scriptling

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/scriptling/conversion"
	"github.com/paularlott/scriptling/errors"
	"github.com/paularlott/scriptling/object"
)

// maxRawBytes caps one raw transfer: a script holds file content in memory.
const maxRawBytes = 64 << 20

// GetApiClientLibrary returns the knot.apiclient Go transport library.
// In embedded contexts this replaces the Python apiclient.py transport stub.
// configure() and is_configured() are no-ops - the client is pre-configured.
// Tokens are never exposed to scripts.
func GetApiClientLibrary(client rest.RESTClient, userId string) *object.Library {
	builder := object.NewLibraryBuilder("knot.apiclient", "Knot API transport (embedded)")

	builder.FunctionWithHelp("configure", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return object.NewBoolean(true)
	}, "configure(url, token, insecure=False) - No-op in embedded mode; client is pre-configured")

	builder.FunctionWithHelp("is_configured", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return object.NewBoolean(true)
	}, "is_configured() - Always returns True in embedded mode")

	builder.FunctionWithHelp("get", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		if err := errors.MinArgs(args, 1); err != nil {
			return err
		}
		path, err := args[0].AsString()
		if err != nil {
			return errors.ParameterError("path", err)
		}

		// Append query params if provided. The Python transport's signature is
		// get(path, params=None), so a positional dict is honoured too — the
		// knot.* libs (space.list, skill.search, ...) all pass it positionally.
		paramsObj := kwargs.Get("params")
		if paramsObj == nil && len(args) > 1 {
			if dict, ok := args[1].(*object.Dict); ok {
				paramsObj = dict
			}
		}
		if paramsObj != nil {
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
		statusCode, apiErr := client.GetJSON(context.Background(), path, &result)
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
		statusCode, apiErr := client.PostJSON(context.Background(), path, body, &result, 0)
		if apiErr != nil && apiErr != io.EOF {
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
		statusCode, apiErr := client.PutJSON(context.Background(), path, body, &result, 0)
		if apiErr != nil && apiErr != io.EOF {
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
		statusCode, apiErr := client.Delete(context.Background(), path, nil, &result, 0)
		if apiErr != nil && apiErr != io.EOF {
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

	// Raw transfers carry file content, which is bytes rather than JSON.
	rawClient := func() (*rest.HTTPClient, object.Object) {
		hc, ok := client.(*rest.HTTPClient)
		if !ok {
			return nil, &object.Error{Message: "API error: raw transfers need an HTTP client"}
		}
		return hc, nil
	}

	builder.FunctionWithHelp("get_bytes", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		if err := errors.MinArgs(args, 1); err != nil {
			return err
		}
		path, err := args[0].AsString()
		if err != nil {
			return errors.ParameterError("path", err)
		}
		hc, errObj := rawClient()
		if errObj != nil {
			return errObj
		}

		resp, rerr := hc.DoRaw(ctx, http.MethodGet, path, nil, 0, nil)
		if rerr != nil {
			return &object.Error{Message: fmt.Sprintf("API error: %v", rerr)}
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			return &object.Error{Message: fmt.Sprintf("API error: %v", rest.DecodeResponse(resp, nil))}
		}
		body, rerr := io.ReadAll(io.LimitReader(resp.Body, maxRawBytes+1))
		if rerr != nil {
			return &object.Error{Message: fmt.Sprintf("API error: %v", rerr)}
		}
		if len(body) > maxRawBytes {
			return &object.Error{Message: fmt.Sprintf("API error: the response is larger than the %d MB a script may read", maxRawBytes>>20)}
		}
		return object.NewBytes(body)
	}, "get_bytes(path) - Make a GET request, returns the response body as Bytes")

	builder.FunctionWithHelp("put_bytes", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		if err := errors.MinArgs(args, 2); err != nil {
			return err
		}
		path, err := args[0].AsString()
		if err != nil {
			return errors.ParameterError("path", err)
		}
		data, errObj := conversion.ToBytes(args[1])
		if errObj != nil {
			return errObj
		}
		if len(data) > maxRawBytes {
			return &object.Error{Message: fmt.Sprintf("API error: a script may write at most %d MB at a time", maxRawBytes>>20)}
		}
		contentType := "application/octet-stream"
		if ct := kwargs.Get("content_type"); ct != nil {
			if s, err := ct.AsString(); err == nil && s != "" {
				contentType = s
			}
		} else if len(args) > 2 {
			if s, err := args[2].AsString(); err == nil && s != "" {
				contentType = s
			}
		}
		hc, errObj := rawClient()
		if errObj != nil {
			return errObj
		}

		resp, rerr := hc.DoRaw(ctx, http.MethodPut, path, bytes.NewReader(data), int64(len(data)), map[string]string{"Content-Type": contentType})
		if rerr != nil {
			return &object.Error{Message: fmt.Sprintf("API error: %v", rerr)}
		}
		defer resp.Body.Close()

		var result interface{}
		if rerr := rest.DecodeResponse(resp, &result); rerr != nil {
			return &object.Error{Message: fmt.Sprintf("API error: %v", rerr)}
		}
		if result == nil {
			return &object.Null{}
		}
		return conversion.FromGo(result)
	}, "put_bytes(path, data, content_type=\"\") - Make a PUT request with a str or Bytes body, returns Dict or List")

	return builder.Build()
}
