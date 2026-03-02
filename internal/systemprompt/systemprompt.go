package systemprompt

import (
	_ "embed"
	"os"
)

//go:embed system-prompt.md
var systemPrompt string

// GetSystemPrompt returns either the embedded system prompt or loads from file
func GetSystemPrompt(systemPromptFile string) string {
	if systemPromptFile != "" {
		if content, err := os.ReadFile(systemPromptFile); err == nil {
			return string(content)
		}
	}
	return systemPrompt
}

// GetInternalSystemPrompt returns the embedded system prompt (for scaffold command)
func GetInternalSystemPrompt() string {
	return systemPrompt
}