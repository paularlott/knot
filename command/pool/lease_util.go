package command_pool

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command/cmdutil"
)

// parseLeaseDuration converts a --time flag value into the API's
// duration_seconds: "" → 0 (use the pool's configured maximum), "none" → -1
// (never expire; only valid on unlimited pools), otherwise a Go duration
// string like "5m" or "90s".
func parseLeaseDuration(value string) (int, error) {
	if value == "" {
		return 0, nil
	}
	if value == "none" || value == "never" {
		return -1, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q (use e.g. 90s, 5m, 2h, or none)", value)
	}
	seconds := int(duration.Seconds())
	if seconds <= 0 {
		return 0, fmt.Errorf("duration must be positive")
	}
	return seconds, nil
}

// formatLeaseExpiry renders a lease expiry for CLI output.
func formatLeaseExpiry(expiresAt *time.Time) string {
	if expiresAt == nil {
		return "never"
	}
	return expiresAt.Local().Format(time.RFC3339)
}

// cleanAPIError strips the transport framing the REST client wraps around
// non-2xx responses and returns just the server's error message.
func cleanAPIError(err error) string {
	return cmdutil.CleanAPIError(err)
}

// leaseErrorJSON is the structured failure emitted in --json mode so pipes
// can branch on it: {"state":"error","error":"…","code":400}.
type leaseErrorJSON struct {
	State string `json:"state"`
	Error string `json:"error"`
	Code  int    `json:"code,omitempty"`
}

// handleLeaseError reports a failed lease command. In --json mode it writes
// the structured error to stdout — keeping the pipe parseable — and returns
// a clean error for stderr and the exit status; otherwise it returns the
// clean error (or humanFallback) for normal display.
func handleLeaseError(cmd *cli.Command, code int, err error, humanFallback string) error {
	message := cleanAPIError(err)
	if message == err.Error() && humanFallback != "" {
		message = humanFallback
	}
	if cmd.GetBool("json") {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if encErr := enc.Encode(leaseErrorJSON{State: "error", Error: message, Code: code}); encErr != nil {
			return encErr
		}
	}
	return errors.New(message)
}

// printLeaseJSON emits the raw lease structure for piping into other tools.
func printLeaseJSON(lease *apiclient.LeaseInfo) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(lease)
}

// leaseJSONFlag is the shared --json flag for the lease commands.
func leaseJSONFlag() cli.Flag {
	return &cli.BoolFlag{
		Name:  "json",
		Usage: "Emit the raw structured result as JSON; failures emit {state, error, code}",
	}
}
