package scriptling

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/paularlott/scriptling/errors"
	"github.com/paularlott/scriptling/object"
)

// HealthCheckResult holds the result of a health check execution.
type HealthCheckResult struct {
	Healthy bool `json:"healthy" msgpack:"healthy"`
}

// ParseHealthCheckResult extracts a HealthCheckResult from a script exception message.
func ParseHealthCheckResult(msg string) (*HealthCheckResult, bool) {
	if !strings.Contains(msg, "\"healthy\"") {
		return nil, false
	}
	var result HealthCheckResult
	if err := json.Unmarshal([]byte(msg), &result); err != nil {
		return nil, false
	}
	return &result, true
}

func healthCheckExit(healthy bool) object.Object {
	data, _ := json.Marshal(HealthCheckResult{Healthy: healthy})
	return &object.Exception{
		Message:       string(data),
		ExceptionType: object.ExceptionTypeSystemExit,
		Code:          0,
	}
}

// healthCheckArg returns the argument at position i, or the keyword argument
// name when it is passed by keyword instead, or nil when neither is given.
func healthCheckArg(args []object.Object, kwargs object.Kwargs, i int, name string) object.Object {
	if v := kwargs.Get(name); v != nil {
		return v
	}
	if len(args) > i {
		return args[i]
	}
	return nil
}

// GetHealthCheckLibrary returns the knot.healthcheck built-in library.
func GetHealthCheckLibrary() *object.Library {
	builder := object.NewLibraryBuilder("knot.healthcheck", "Health check functions for space monitoring")

	builder.FunctionWithHelp("http_head", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		urlArg := healthCheckArg(args, kwargs, 0, "url")
		if urlArg == nil {
			return errors.NewError("http_head: url argument required")
		}
		url, err := urlArg.AsString()
		if err != nil {
			return errors.NewError("http_head: url must be a string")
		}
		skipSSL := false
		timeout := 10
		if v := healthCheckArg(args, kwargs, 1, "skip_ssl_verify"); v != nil {
			b, e := v.AsBool()
			if e != nil {
				return errors.NewError("http_head: skip_ssl_verify must be a bool")
			}
			skipSSL = b
		}
		if v := healthCheckArg(args, kwargs, 2, "timeout"); v != nil {
			t, e := v.AsInt()
			if e != nil {
				return errors.NewError("http_head: timeout must be an int")
			}
			timeout = int(t)
		}

		transport := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: skipSSL},
		}
		client := &http.Client{
			Timeout:   time.Duration(timeout) * time.Second,
			Transport: transport,
		}
		resp, httpErr := client.Head(url)
		if httpErr != nil {
			return object.NewBoolean(false)
		}
		resp.Body.Close()
		return object.NewBoolean(resp.StatusCode == http.StatusOK)
	}, "http_head(url, skip_ssl_verify=False, timeout=10) - HTTP HEAD check, returns True if status 200")

	builder.FunctionWithHelp("tcp_port", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		portArg := healthCheckArg(args, kwargs, 0, "port")
		if portArg == nil {
			return errors.NewError("tcp_port: port argument required")
		}
		port, err := portArg.AsInt()
		if err != nil {
			return errors.NewError("tcp_port: port must be an int")
		}
		timeout := 10
		if v := healthCheckArg(args, kwargs, 1, "timeout"); v != nil {
			t, e := v.AsInt()
			if e != nil {
				return errors.NewError("tcp_port: timeout must be an int")
			}
			timeout = int(t)
		}

		conn, dialErr := net.DialTimeout("tcp", strings.Join([]string{"127.0.0.1:", fmt.Sprint(port)}, ""), time.Duration(timeout)*time.Second)
		if dialErr != nil {
			return object.NewBoolean(false)
		}
		conn.Close()
		return object.NewBoolean(true)
	}, "tcp_port(port, timeout=10) - TCP port check, returns True if port is open")

	builder.FunctionWithHelp("program", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		commandArg := healthCheckArg(args, kwargs, 0, "command")
		if commandArg == nil {
			return errors.NewError("program: command argument required")
		}
		command, err := commandArg.AsString()
		if err != nil {
			return errors.NewError("program: command must be a string")
		}
		timeout := 10
		if v := healthCheckArg(args, kwargs, 1, "timeout"); v != nil {
			t, e := v.AsInt()
			if e != nil {
				return errors.NewError("program: timeout must be an int")
			}
			timeout = int(t)
		}

		parts := strings.Fields(command)
		if len(parts) == 0 {
			return object.NewBoolean(false)
		}
		cmdCtx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
		defer cancel()
		cmd := exec.CommandContext(cmdCtx, parts[0], parts[1:]...)
		runErr := cmd.Run()
		return object.NewBoolean(runErr == nil)
	}, "program(command, timeout=10) - Run command, returns True if exit code 0")

	builder.FunctionWithHelp("check_result", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		if len(args) < 1 {
			return errors.NewError("check_result: healthy argument required")
		}
		healthy, err := args[0].AsBool()
		if err != nil {
			return errors.NewError("check_result: argument must be a bool")
		}
		return healthCheckExit(healthy)
	}, "check_result(healthy) - Report health check result and exit")

	return builder.Build()
}
