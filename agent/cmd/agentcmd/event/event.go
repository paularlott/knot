package event

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/paularlott/cli"
)

func agentAPIPort() int {
	if v := os.Getenv("KNOT_API_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			return p
		}
	}
	return 12201
}

var EventCmd = &cli.Command{
	Name:        "event",
	Usage:       "Emit a custom event",
	Description: "Emit a custom event from this space. The event type gets a 'custom.' prefix automatically. Payload is a JSON string or read from stdin if omitted.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "type",
			Usage:    "The event type (e.g. myapp.deployed)",
			Required: true,
		},
		&cli.StringArg{
			Name:     "payload",
			Usage:    "JSON payload string (reads from stdin if omitted)",
			Required: false,
		},
	},
	MaxArgs: 2,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		eventType := cmd.GetStringArg("type")
		payloadStr := cmd.GetStringArg("payload")

		var payload map[string]interface{}

		if payloadStr != "" {
			if err := json.Unmarshal([]byte(payloadStr), &payload); err != nil {
				return fmt.Errorf("invalid JSON payload: %w", err)
			}
		} else {
			stat, _ := os.Stdin.Stat()
			if (stat.Mode() & os.ModeCharDevice) == 0 {
				data, err := io.ReadAll(os.Stdin)
				if err != nil {
					return fmt.Errorf("failed to read stdin: %w", err)
				}
				if len(data) > 0 {
					if err := json.Unmarshal(data, &payload); err != nil {
						return fmt.Errorf("invalid JSON from stdin: %w", err)
					}
				}
			}
		}

		req := map[string]interface{}{
			"type":    eventType,
			"payload": payload,
		}

		body, err := json.Marshal(req)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}

		url := fmt.Sprintf("http://127.0.0.1:%d/event", agentAPIPort())
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(httpReq)
		if err != nil {
			return fmt.Errorf("failed to emit event: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusAccepted {
			respBody, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("agent rejected event (status %d): %s", resp.StatusCode, string(respBody))
		}

		fmt.Printf("Event 'custom.%s' emitted.\n", eventType)
		return nil
	},
}
