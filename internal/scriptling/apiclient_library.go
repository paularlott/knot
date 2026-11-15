package scriptling

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/scriptling/conversion"
	"github.com/paularlott/scriptling/errors"
	"github.com/paularlott/scriptling/object"
)

// maxRawBytes caps one raw transfer: a script holds file content in memory.
const maxRawBytes = 64 << 20

// Caps on what a script hands the transport, checked on the script value
// before it is converted to Go: a value shared by many containers converts
// (and encodes) once per reference, so a few kilobytes of script data could
// otherwise expand into gigabytes. maxRequestBodyBytes is above the largest
// value the API takes (a 10 MB variable).
const (
	maxRequestBodyBytes   = 16 << 20
	maxRequestHeaderBytes = 64 << 10
	maxQueryBytes         = 1 << 20
)

// RequestPrechecker is a client that refuses some calls on their method and
// path alone (the AI script client). The transport asks it before it
// converts a request's body, headers or query parameters, so a call that
// will be refused costs nothing to build.
type RequestPrechecker interface {
	PrecheckRequest(method, path string) error
}

// precheck asks the client, when it is a RequestPrechecker, whether the call
// may go ahead; nil when it may.
func precheck(client rest.RESTClient, method, path string) object.Object {
	if pc, ok := client.(RequestPrechecker); ok {
		if err := pc.PrecheckRequest(method, path); err != nil {
			return &object.Error{Message: fmt.Sprintf("API error: %v", err)}
		}
	}
	return nil
}

// boundedToGo converts a script value for a request, refusing it when it
// would encode to more than limit bytes (what names it in the error) and
// holding the conversion to the interpreter's own limits.
func boundedToGo(obj object.Object, limit int, what string) (interface{}, object.Object) {
	if size := JSONSizeAtLeast(obj, limit); size > limit {
		return nil, &object.Error{Message: fmt.Sprintf("API error: the %s is larger than the %d bytes a request may carry", what, limit)}
	}
	v, errObj := conversion.ToGoLimited(obj)
	if errObj != nil {
		return nil, errObj
	}
	return v, nil
}

// stringHeaders converts a headers dict, bounded like a body.
func stringHeaders(obj object.Object) (map[string]string, object.Object) {
	headers := map[string]string{}
	if obj == nil {
		return headers, nil
	}
	v, errObj := boundedToGo(obj, maxRequestHeaderBytes, "request headers")
	if errObj != nil {
		return nil, errObj
	}
	if d, ok := v.(map[string]interface{}); ok {
		for k, val := range d {
			headers[k] = fmt.Sprint(val)
		}
	}
	return headers, nil
}

// JSONSizeAtLeast returns a lower bound on the size of obj encoded as JSON,
// giving up once it passes limit (the result is then over limit): strings
// count their bytes and quotes, containers their brackets and separators,
// and every other value one byte. A value shared by several containers
// counts once per reference, as it would encode. A container that holds
// itself stops counting once the limit is passed, as each level adds at
// least two bytes.
func JSONSizeAtLeast(obj object.Object, limit int) int {
	size := 0
	var walk func(o object.Object)
	walkAll := func(elements []object.Object) {
		size += 2 + len(elements)
		for _, e := range elements {
			if size > limit {
				return
			}
			walk(e)
		}
	}
	walk = func(o object.Object) {
		if size > limit {
			return
		}
		switch v := o.(type) {
		case *object.String:
			size += len(v.StringValue()) + 2
		case *object.Bytes:
			size += v.Len() + 2
		case *object.List:
			walkAll(v.Elements)
		case *object.Tuple:
			walkAll(v.Elements)
		case *object.Set:
			size += 2 + len(v.Elements)
			for _, e := range v.Elements {
				if size > limit {
					return
				}
				walk(e)
			}
		case *object.FloatArray:
			size += 2 + 2*len(v.Data)
		case *object.Dict:
			size += 2 + 2*len(v.Pairs)
			for _, pair := range v.Pairs {
				if size > limit {
					return
				}
				walk(pair.Key)
				walk(pair.Value)
			}
		default:
			size++
		}
	}
	walk(obj)
	return size
}

// callContext bounds one call by the timeout, in seconds, the script gives
// as timeout= or as the positional argument at pos, lifting the REST
// client's own timeout so a longer one is honoured. A call given no timeout
// keeps the client's: none in the server, six minutes in a space.
func callContext(ctx context.Context, kwargs object.Kwargs, args []object.Object, pos int) (context.Context, context.CancelFunc, object.Object) {
	t := kwargs.Get("timeout")
	if t == nil && len(args) > pos {
		t = args[pos]
	}
	if t == nil {
		return ctx, func() {}, nil
	}
	if _, isNull := t.(*object.Null); isNull {
		return ctx, func() {}, nil
	}
	secs, err := t.AsFloat()
	if err != nil {
		return nil, nil, errors.ParameterError("timeout", err)
	}
	if secs <= 0 {
		return nil, nil, &object.Error{Message: "timeout must be more than 0 seconds"}
	}
	ctx, cancel := context.WithTimeout(rest.WithoutTimeout(ctx), time.Duration(secs*float64(time.Second)))
	return ctx, cancel, nil
}

// argOrKwarg returns the argument given as name= or at position pos,
// matching the Python transport's signatures; nil when absent or None.
func argOrKwarg(args []object.Object, kwargs object.Kwargs, pos int, name string) object.Object {
	v := kwargs.Get(name)
	if v == nil && len(args) > pos {
		v = args[pos]
	}
	if v == nil {
		return nil
	}
	if _, isNull := v.(*object.Null); isNull {
		return nil
	}
	return v
}

// statusError is the error for a refused request, with the server's own
// message when the response carried one, as the in-process client leaves
// it decoded in result.
func statusError(statusCode int, result interface{}) object.Object {
	if m, ok := result.(map[string]interface{}); ok {
		if msg, ok := m["error"].(string); ok && msg != "" {
			return &object.Error{Message: fmt.Sprintf("API error: HTTP %d: %s", statusCode, msg)}
		}
		// A JSON-RPC error (/api/methods/call) carries its message in an object.
		if e, ok := m["error"].(map[string]interface{}); ok {
			if msg, ok := e["message"].(string); ok && msg != "" {
				return &object.Error{Message: fmt.Sprintf("API error: HTTP %d: %s", statusCode, msg)}
			}
		}
	}
	return &object.Error{Message: fmt.Sprintf("API error: HTTP %d", statusCode)}
}

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
		if errObj := precheck(client, http.MethodGet, path); errObj != nil {
			return errObj
		}
		if paramsObj != nil {
			if paramsDict, ok := paramsObj.(*object.Dict); ok {
				if size := JSONSizeAtLeast(paramsDict, maxQueryBytes); size > maxQueryBytes {
					return &object.Error{Message: fmt.Sprintf("API error: the query parameters are larger than the %d bytes a request may carry", maxQueryBytes)}
				}
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

		ctx, cancel, errObj := callContext(ctx, kwargs, args, 2)
		if errObj != nil {
			return errObj
		}
		defer cancel()

		var result interface{}
		statusCode, apiErr := client.GetJSON(ctx, path, &result)
		// An empty 2xx body is no error: it returns None, as post, put
		// and delete do.
		if apiErr != nil && apiErr != io.EOF {
			return &object.Error{Message: fmt.Sprintf("API error: %v", apiErr)}
		}
		if statusCode >= 400 {
			return statusError(statusCode, result)
		}
		if result == nil {
			return &object.Null{}
		}
		return conversion.FromGo(result)
	}, "get(path, params=None, timeout=None) - Make a GET request, waiting up to timeout seconds if given; returns Dict or List")

	builder.FunctionWithHelp("post", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		if err := errors.MinArgs(args, 1); err != nil {
			return err
		}
		path, err := args[0].AsString()
		if err != nil {
			return errors.ParameterError("path", err)
		}

		if errObj := precheck(client, http.MethodPost, path); errObj != nil {
			return errObj
		}
		var body interface{}
		if b := argOrKwarg(args, kwargs, 1, "body"); b != nil {
			var errObj object.Object
			if body, errObj = boundedToGo(b, maxRequestBodyBytes, "request body"); errObj != nil {
				return errObj
			}
		}

		ctx, cancel, errObj := callContext(ctx, kwargs, args, 2)
		if errObj != nil {
			return errObj
		}
		defer cancel()

		// Extra request headers, such as the Content-Type a file upload in
		// parts records for the file.
		headers, errObj := stringHeaders(argOrKwarg(args, kwargs, 3, "headers"))
		if errObj != nil {
			return errObj
		}

		var result interface{}
		if len(headers) > 0 {
			hc, ok := client.(rest.RawClient)
			if !ok {
				return &object.Error{Message: "API error: request headers are not supported by this client"}
			}
			var reader io.Reader
			var size int64
			if body != nil {
				data, merr := json.Marshal(body)
				if merr != nil {
					return &object.Error{Message: fmt.Sprintf("API error: %v", merr)}
				}
				reader, size = bytes.NewReader(data), int64(len(data))
			}
			if _, set := headers["Content-Type"]; !set {
				headers["Content-Type"] = "application/json"
			}
			resp, rerr := hc.DoRaw(ctx, http.MethodPost, path, reader, size, headers)
			if rerr != nil {
				return &object.Error{Message: fmt.Sprintf("API error: %v", rerr)}
			}
			defer resp.Body.Close()
			if derr := rest.DecodeResponse(resp, &result); derr != nil && derr != io.EOF {
				return &object.Error{Message: fmt.Sprintf("API error: %v", derr)}
			}
		} else {
			statusCode, apiErr := client.PostJSON(ctx, path, body, &result, 0)
			if apiErr != nil && apiErr != io.EOF {
				return &object.Error{Message: fmt.Sprintf("API error: %v", apiErr)}
			}
			if statusCode >= 400 {
				return statusError(statusCode, result)
			}
		}
		if result == nil {
			return &object.Null{}
		}
		return conversion.FromGo(result)
	}, "post(path, body=None, timeout=None, headers=None) - Make a POST request, with any extra request headers, waiting up to timeout seconds if given; returns Dict or List")

	builder.FunctionWithHelp("put", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		if err := errors.MinArgs(args, 1); err != nil {
			return err
		}
		path, err := args[0].AsString()
		if err != nil {
			return errors.ParameterError("path", err)
		}

		if errObj := precheck(client, http.MethodPut, path); errObj != nil {
			return errObj
		}
		var body interface{}
		if b := argOrKwarg(args, kwargs, 1, "body"); b != nil {
			var errObj object.Object
			if body, errObj = boundedToGo(b, maxRequestBodyBytes, "request body"); errObj != nil {
				return errObj
			}
		}

		ctx, cancel, errObj := callContext(ctx, kwargs, args, 2)
		if errObj != nil {
			return errObj
		}
		defer cancel()

		var result interface{}
		statusCode, apiErr := client.PutJSON(ctx, path, body, &result, 0)
		if apiErr != nil && apiErr != io.EOF {
			return &object.Error{Message: fmt.Sprintf("API error: %v", apiErr)}
		}
		if statusCode >= 400 {
			return statusError(statusCode, result)
		}
		if result == nil {
			return &object.Null{}
		}
		return conversion.FromGo(result)
	}, "put(path, body=None, timeout=None) - Make a PUT request, waiting up to timeout seconds if given; returns Dict or List")

	builder.FunctionWithHelp("delete", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		if err := errors.MinArgs(args, 1); err != nil {
			return err
		}
		path, err := args[0].AsString()
		if err != nil {
			return errors.ParameterError("path", err)
		}

		if errObj := precheck(client, http.MethodDelete, path); errObj != nil {
			return errObj
		}
		// Extra request headers, such as If-Match for a conditional delete.
		headers, errObj := stringHeaders(argOrKwarg(args, kwargs, 1, "headers"))
		if errObj != nil {
			return errObj
		}

		ctx, cancel, errObj := callContext(ctx, kwargs, args, 2)
		if errObj != nil {
			return errObj
		}
		defer cancel()

		var result interface{}
		if len(headers) > 0 {
			hc, ok := client.(rest.RawClient)
			if !ok {
				return &object.Error{Message: "API error: request headers are not supported by this client"}
			}
			resp, rerr := hc.DoRaw(ctx, http.MethodDelete, path, nil, 0, headers)
			if rerr != nil {
				return &object.Error{Message: fmt.Sprintf("API error: %v", rerr)}
			}
			defer resp.Body.Close()
			if derr := rest.DecodeResponse(resp, &result); derr != nil && derr != io.EOF {
				return &object.Error{Message: fmt.Sprintf("API error: %v", derr)}
			}
		} else {
			statusCode, apiErr := client.Delete(ctx, path, nil, &result, 0)
			if apiErr != nil && apiErr != io.EOF {
				return &object.Error{Message: fmt.Sprintf("API error: %v", apiErr)}
			}
			if statusCode >= 400 {
				return statusError(statusCode, result)
			}
		}
		if result == nil {
			return &object.Null{}
		}
		return conversion.FromGo(result)
	}, "delete(path, headers=None, timeout=None) - Make a DELETE request, with any extra request headers, waiting up to timeout seconds if given; returns Dict or List")

	// Raw transfers carry file content, which is bytes rather than JSON.
	rawClient := func() (rest.RawClient, object.Object) {
		hc, ok := client.(rest.RawClient)
		if !ok {
			return nil, &object.Error{Message: "API error: raw transfers are not supported by this client"}
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

		ctx, cancel, errObj := callContext(ctx, kwargs, args, 1)
		if errObj != nil {
			return errObj
		}
		defer cancel()

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
	}, "get_bytes(path, timeout=None) - Make a GET request, waiting up to timeout seconds if given; returns the response body as Bytes")

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

		ctx, cancel, errObj := callContext(ctx, kwargs, args, 3)
		if errObj != nil {
			return errObj
		}
		defer cancel()

		resp, rerr := hc.DoRaw(ctx, http.MethodPut, path, bytes.NewReader(data), int64(len(data)), map[string]string{"Content-Type": contentType})
		if rerr != nil {
			return &object.Error{Message: fmt.Sprintf("API error: %v", rerr)}
		}
		defer resp.Body.Close()

		var result interface{}
		// An empty 2xx body returns None, as post, put and delete do.
		if rerr := rest.DecodeResponse(resp, &result); rerr != nil && rerr != io.EOF {
			return &object.Error{Message: fmt.Sprintf("API error: %v", rerr)}
		}
		if result == nil {
			return &object.Null{}
		}
		return conversion.FromGo(result)
	}, "put_bytes(path, data, content_type=\"\", timeout=None) - Make a PUT request with a str or Bytes body, waiting up to timeout seconds if given; returns Dict or List")

	return builder.Build()
}
