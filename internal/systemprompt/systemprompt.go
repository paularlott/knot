package systemprompt

import (
	_ "embed"
	"os"
)

//go:embed system-prompt.md
var onDemandSystemPrompt string

//go:embed system-prompt-native-tools.md
var nativeToolsSystemPrompt string

var currentDefaultPrompt string = onDemandSystemPrompt

// SetDefaultSystemPrompt sets which embedded prompt to use as the default
func SetDefaultSystemPrompt(useNativeTools bool) {
	if useNativeTools {
		currentDefaultPrompt = nativeToolsSystemPrompt
	} else {
		currentDefaultPrompt = onDemandSystemPrompt
	}
}

// GetSystemPrompt returns either the current default system prompt or loads from file
func GetSystemPrompt(systemPromptFile string) string {
	systemPrompt := currentDefaultPrompt
	if systemPromptFile != "" {
		if content, err := os.ReadFile(systemPromptFile); err == nil {
			systemPrompt = string(content)
		}
	}
	return systemPrompt
}

// GetInternalSystemPrompt returns the embedded on-demand system prompt (for scaffold command)
func GetInternalSystemPrompt() string {
	return onDemandSystemPrompt
}

// GetInternalSystemPromptNative returns the embedded native tools system prompt
func GetInternalSystemPromptNative() string {
	return nativeToolsSystemPrompt
}