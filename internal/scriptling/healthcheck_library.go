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
	Healthy bool `json:"healthy"`
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

// GetHealthCheckLibrary returns the knot.healthcheck built-in library.
func GetHealthCheckLibrary() *object.Library {
	builder := object.NewLibraryBuilder("knot.healthcheck", "Health check functions for space monitoring")

	builder.FunctionWithHelp("http_head", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		if len(args) < 1 {
			return errors.NewError("http_head: url argument required")
		}
		url, err := args[0].AsString()
		if err != nil {
			return errors.NewError("http_head: url must be a string")
		}
		skipSSL := false
		timeout := 10
		if len(args) >= 2 {
			b, e := args[1].AsBool()
			if e != nil {
				return errors.NewError("http_head: skip_ssl_verify must be a bool")
			}
			skipSSL = b
		}
		if len(args) >= 3 {
			t, e := args[2].AsInt()
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
			return &object.Boolean{Value: false}
		}
		resp.Body.Close()
		return &object.Boolean{Value: resp.StatusCode == http.StatusOK}
	}, "http_head(url, skip_ssl_verify=False, timeout=10) - HTTP HEAD check, returns True if status 200")

	builder.FunctionWithHelp("tcp_port", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		if len(args) < 1 {
			return errors.NewError("tcp_port: port argument required")
		}
		port, err := args[0].AsInt()
		if err != nil {
			return errors.NewError("tcp_port: port must be an int")
		}
		timeout := 10
		if len(args) >= 2 {
			t, e := args[1].AsInt()
			if e != nil {
				return errors.NewError("tcp_port: timeout must be an int")
			}
			timeout = int(t)
		}

		conn, dialErr := net.DialTimeout("tcp", strings.Join([]string{"127.0.0.1:", fmt.Sprint(port)}, ""), time.Duration(timeout)*time.Second)
		if dialErr != nil {
			return &object.Boolean{Value: false}
		}
		conn.Close()
		return &object.Boolean{Value: true}
	}, "tcp_port(port, timeout=10) - TCP port check, returns True if port is open")

	builder.FunctionWithHelp("program", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		if len(args) < 1 {
			return errors.NewError("program: command argument required")
		}
		command, err := args[0].AsString()
		if err != nil {
			return errors.NewError("program: command must be a string")
		}
		timeout := 10
		if len(args) >= 2 {
			t, e := args[1].AsInt()
			if e != nil {
				return errors.NewError("program: timeout must be an int")
			}
			timeout = int(t)
		}

		parts := strings.Fields(command)
		if len(parts) == 0 {
			return &object.Boolean{Value: false}
		}
		cmdCtx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
		defer cancel()
		cmd := exec.CommandContext(cmdCtx, parts[0], parts[1:]...)
		runErr := cmd.Run()
		return &object.Boolean{Value: runErr == nil}
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
