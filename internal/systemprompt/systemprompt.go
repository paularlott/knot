package systemprompt

import (
	_ "embed"
	"os"
)

//go:embed system-prompt.md
var defaultSystemPrompt string

// GetSystemPrompt returns either the built-in system prompt or loads from file
func GetSystemPrompt(systemPromptFile string) string {
	systemPrompt := defaultSystemPrompt
	if systemPromptFile != "" {
		if content, err := os.ReadFile(systemPromptFile); err == nil {
			systemPrompt = string(content)
		}
	}
	return systemPrompt
}

// GetInternalSystemPrompt returns the embedded system prompt (for scaffold command)
func GetInternalSystemPrompt() string {
	return defaultSystemPrompt
}