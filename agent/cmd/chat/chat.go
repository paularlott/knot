package command_chat

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/util/rest"

	"github.com/paularlott/cli"
)

// ANSI color codes
const (
	ColorReset  = "\033[0m"
	ColorBold   = "\033[1m"
	ColorBlue   = "\033[34m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorCyan   = "\033[36m"
	ColorGray   = "\033[90m"
	ColorRed    = "\033[31m"
)

type ChatMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
}

type ChatRequest struct {
	Messages []ChatMessage `json:"messages"`
}

type SSEEvent struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

var ChatCmd = &cli.Command{
	Name:  "chat",
	Usage: "Start an interactive chat session with the AI assistant",
	Description: `The chat command allows you to have an interactive conversation with the AI assistant.

Type your messages and press Enter to send them. The assistant will respond in real-time.
Type 'exit' or 'quit' to end the session.`,
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
		&cli.BoolFlag{
			Name:         "show-thinking",
			Usage:        "Show the AI's thinking process instead of animation.",
			DefaultValue: false,
		},
	},
	Run: func(ctx context.Context, cmd *cli.Command) error {
		alias := cmd.GetString("alias")
		cfg := config.GetServerAddr(alias, cmd)

		// Create REST client
		client, err := rest.NewClient(cfg.HttpServer, cfg.ApiToken, cmd.GetBool("tls-skip-verify"))
		if err != nil {
			return fmt.Errorf("failed to create REST client: %w", err)
		}
		client.SetTimeout(5 * time.Minute)
		client.SetHeader("X-Knot-Api-Version", "2025-03-10")

		fmt.Printf("%sKnot AI Assistant%s\n", ColorBold+ColorCyan, ColorReset)
		fmt.Printf("%sType your message and press Enter. Type 'exit' or 'quit' to end the session.%s\n", ColorGray, ColorReset)
		fmt.Println()

		scanner := bufio.NewScanner(os.Stdin)
		var messages []ChatMessage

		for {
			fmt.Printf("%s%sYou:%s ", ColorBold, ColorBlue, ColorReset)
			if !scanner.Scan() {
				break
			}

			input := strings.TrimSpace(scanner.Text())
			if input == "" {
				continue
			}

			if input == "exit" || input == "quit" {
				fmt.Printf("%sGoodbye! 👋%s\n", ColorYellow, ColorReset)
				break
			}

			// Add user message
			messages = append(messages, ChatMessage{
				Role:      "user",
				Content:   input,
				Timestamp: time.Now().Unix(),
			})

			// Send request and stream response
			assistantMessage, err := sendChatRequest(client, messages, cmd.GetBool("show-thinking"))
			if err != nil {
				fmt.Printf("%sError:%s %v\n", ColorRed, ColorReset, err)
				continue
			}

			// Add assistant response to conversation history
			assistantMessage = strings.TrimSpace(assistantMessage)
			if assistantMessage != "" {
				// Strip think tags to prevent LLM template errors (like web chat does)
				cleanContent := stripThinkTags(assistantMessage)
				if cleanContent != "" {
					messages = append(messages, ChatMessage{
						Role:      "assistant",
						Content:   cleanContent,
						Timestamp: time.Now().Unix(),
					})
				}
			}

			fmt.Println()
		}

		return nil
	},
}

func sendChatRequest(client *rest.RESTClient, messages []ChatMessage, showThinking bool) (string, error) {
	chatReq := ChatRequest{Messages: messages}

	fmt.Printf("%s%sAssistant:%s ", ColorBold, ColorGreen, ColorReset)

	var fullResponse strings.Builder
	var thinkingActive bool
	var thinkingTicker *time.Ticker
	var thinkingDone chan bool

	err := rest.StreamData(
		client,
		context.Background(),
		"POST",
		"api/chat/stream",
		chatReq,
		func(event *SSEEvent) (bool, error) {
			switch event.Type {
			case "content":
				if content, ok := event.Data.(string); ok {
					if showThinking {
						// Show thinking content with colors
						if strings.Contains(content, "<think>") {
							content = strings.ReplaceAll(content, "<think>", "\n\n"+ColorGray+"<think>\n")
						}
						if strings.Contains(content, "</think>") {
							content = strings.ReplaceAll(content, "</think>", "\n</think>"+ColorReset+"\n\n")
						}
						fmt.Print(content)
					} else {
						// Handle thinking animation
						if strings.Contains(content, "<think>") {
							if !thinkingActive {
								thinkingActive = true
								thinkingTicker = time.NewTicker(500 * time.Millisecond)
								thinkingDone = make(chan bool)
								go showThinkingAnimation(thinkingTicker, thinkingDone)
							}
						}
						if strings.Contains(content, "</think>") {
							if thinkingActive {
								thinkingActive = false
								thinkingTicker.Stop()
								thinkingDone <- true
								fmt.Print("\r\033[K") // Clear thinking animation
							}
						}
						// Only print non-thinking content
						if !strings.Contains(content, "<think>") && !strings.Contains(content, "</think>") && !thinkingActive {
							fmt.Print(content)
						}
					}
					fullResponse.WriteString(content)
				}
			case "error":
				if errorData, ok := event.Data.(map[string]interface{}); ok {
					if errorMsg, ok := errorData["error"].(string); ok {
						return true, fmt.Errorf("server error: %s", errorMsg)
					}
				}
			case "done":
				fmt.Println()
				return true, nil
			}
			return false, nil
		},
	)

	// Clean up thinking animation if still active
	if thinkingActive {
		thinkingTicker.Stop()
		thinkingDone <- true
		fmt.Print("\r\033[K") // Clear thinking animation
	}

	if err != nil {
		return "", err
	}

	return fullResponse.String(), nil
}

// showThinkingAnimation displays a thinking animation
func showThinkingAnimation(ticker *time.Ticker, done chan bool) {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	frameIndex := 0

	for {
		select {
		case <-ticker.C:
			fmt.Printf("\r%s%s thinking...%s", ColorGray, frames[frameIndex], ColorReset)
			frameIndex = (frameIndex + 1) % len(frames)
		case <-done:
			return
		}
	}
}

// stripThinkTags removes <think>...</think> tags from content to prevent LLM template errors
func stripThinkTags(content string) string {
	// Use regex to remove think tags and their content
	re := regexp.MustCompile(`<think>[\s\S]*?</think>`)
	return strings.TrimSpace(re.ReplaceAllString(content, ""))
}
