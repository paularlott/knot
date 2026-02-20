package command_chat

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/paularlott/cli"
	"github.com/paularlott/cli/tui"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/util/rest"
	mcpopenai "github.com/paularlott/mcp/ai/openai"
)

type ChatMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
}

var ChatCmd = &cli.Command{
	Name:  "chat",
	Usage: "Start an interactive chat session with the AI assistant",
	Description: `The chat command allows you to have an interactive conversation with the AI assistant.

Type your messages and press Enter to send them. The assistant will respond in real-time.
Type /exit to end the session.`,
	MaxArgs: cli.NoArgs,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "server",
			Aliases: []string{"s"},
			Usage:   "The address of the remote server to connect to.",
			EnvVars: []string{config.CONFIG_ENV_PREFIX + "_SERVER"},
		},
		&cli.StringFlag{
			Name:    "token",
			Aliases: []string{"t"},
			Usage:   "The token to use for authentication.",
			EnvVars: []string{config.CONFIG_ENV_PREFIX + "_TOKEN"},
		},
		&cli.BoolFlag{
			Name:         "tls-skip-verify",
			Usage:        "Skip TLS verification when talking to server.",
			ConfigPath:   []string{"tls.skip_verify"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_TLS_SKIP_VERIFY"},
			DefaultValue: true,
		},
		&cli.StringFlag{
			Name:         "alias",
			Aliases:      []string{"a"},
			Usage:        "The server alias to use.",
			DefaultValue: "default",
		},
	},
	Run: func(ctx context.Context, cmd *cli.Command) error {
		alias := cmd.GetString("alias")
		cfg := config.GetServerAddr(alias, cmd)

		client, err := rest.NewClient(cfg.HttpServer, cfg.ApiToken, cmd.GetBool("tls-skip-verify"))
		if err != nil {
			return fmt.Errorf("failed to create REST client: %w", err)
		}
		client.SetTimeout(5 * time.Minute)
		client.SetHeader("X-Knot-Api-Version", "2025-03-10")

		var t *tui.TUI
		var messages []ChatMessage

		t = tui.New(tui.Config{
			StatusLeft:  cfg.HttpServer,
			StatusRight: "Ctrl+C to exit",
			Commands: []*tui.Command{
				{
					Name:        "exit",
					Description: "Exit the chat session",
					Handler:     func(_ string) { t.Exit() },
				},
				{
					Name:        "clear",
					Description: "Clear conversation history",
					Handler: func(_ string) {
						messages = nil
						t.ClearOutput()
					},
				},
			},
			OnEscape: func() {
				t.StopStreaming()
			},
			OnSubmit: func(text string) {
				t.AddMessage(tui.RoleUser, text)
				messages = append(messages, ChatMessage{
					Role:      "user",
					Content:   text,
					Timestamp: time.Now().Unix(),
				})

				t.StartStreaming()
				t.StartSpinner("Thinking...")

				assistantMsg, err := sendChatRequest(client, messages, t)
				if err != nil {
					t.StopStreaming()
					t.StopSpinner()
					t.AddMessage(tui.RoleSystem, err.Error())
					return
				}

				assistantMsg = strings.TrimSpace(stripThinkTags(assistantMsg))
				if assistantMsg != "" {
					messages = append(messages, ChatMessage{
						Role:      "assistant",
						Content:   assistantMsg,
						Timestamp: time.Now().Unix(),
					})
				}
			},
		})

		t.AddMessage(tui.RoleSystem, "Connected to "+cfg.HttpServer+". Type /exit to quit.")

		return t.Run(ctx)
	},
}

func sendChatRequest(client *rest.HTTPClient, messages []ChatMessage, t *tui.TUI) (string, error) {
	// Convert to OpenAI message format
	openAIMessages := make([]mcpopenai.Message, len(messages))
	for i, msg := range messages {
		openAIMessages[i] = mcpopenai.Message{Role: msg.Role}
		openAIMessages[i].SetContentAsString(msg.Content)
	}

	chatReq := mcpopenai.ChatCompletionRequest{
		Model:    "",
		Messages: openAIMessages,
		Stream:   true,
	}

	var fullResponse strings.Builder
	var spinnerStopped bool

	err := rest.StreamData(
		client,
		context.Background(),
		"POST",
		"v1/chat/completions",
		chatReq,
		func(response *mcpopenai.ChatCompletionResponse) (bool, error) {
			if len(response.Choices) == 0 {
				return false, nil
			}

			content := response.Choices[0].Delta.Content
			if content == "" {
				return false, nil
			}

			if !spinnerStopped {
				spinnerStopped = true
				t.StopSpinner()
			}

			fullResponse.WriteString(content)
			t.StreamChunk(content)
			return false, nil
		},
	)

	if err != nil {
		return "", err
	}

	t.StreamComplete()
	return fullResponse.String(), nil
}

func stripThinkTags(content string) string {
	re := regexp.MustCompile(`(?s)<think[^>]*>.*?</think[^>]*>`)
	return strings.TrimSpace(re.ReplaceAllString(content, ""))
}
