package service

import (
	"os"

	"github.com/paularlott/scriptling/object"
)

// HandleScriptResult processes scriptling evaluation results with consistent exception handling.
// Returns (exitCode, output, error) where:
// - exitCode: 0 for success, non-zero for SystemExit
// - output: captured output or result inspection
// - error: non-nil for errors (excluding successful SystemExit with code 0)
func HandleScriptResult(result object.Object, err error, capturedOutput string) (int, string, error) {
	// Check for SystemExit first (regardless of err value)
	if ex, ok := object.AsException(result); ok && ex.IsSystemExit() {
		exitCode := ex.GetExitCode()
		return exitCode, capturedOutput, nil
	}

	if err != nil {
		return 1, capturedOutput, err
	}

	// Append result if not None
	output := capturedOutput
	if result != nil && result.Inspect() != "None" {
		if output != "" {
			output += "\n"
		}
		output += result.Inspect()
	}

	return 0, output, nil
}

// ExitOnSystemExit checks for SystemExit and exits the process if found
func ExitOnSystemExit(result object.Object) {
	if ex, ok := object.AsException(result); ok && ex.IsSystemExit() {
		os.Exit(ex.GetExitCode())
	}
}
