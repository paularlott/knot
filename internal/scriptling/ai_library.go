package scriptling

import (
	"context"
	"fmt"

	ai "github.com/paularlott/mcp/ai"
	"github.com/paularlott/scriptling/errors"
	scriptlingai "github.com/paularlott/scriptling/extlibs/ai"
	"github.com/paularlott/scriptling/object"
)

var defaultModel string

// SetDefaultModel sets the default model name for the AI library.
// Called from server startup with the configured chat model.
func SetDefaultModel(model string) {
	defaultModel = model
}

// GetAILibrary returns the knot.ai library for scriptling environments.
// It exposes:
//   - Client() - returns the pre-configured AI client
//   - get_default_model() - returns the server-configured default model name
//
// In all environments (MCP, local, remote, streaming), the aiClient connects
// to the server's OpenAI-compatible endpoint with the X-Knot-Passthrough header.
// The MCPServerContext middleware handles per-user tool discovery and execution.
// Scripts can pass model="" to use the server's default model, or specify any
// model explicitly.
//
// When aiClient is nil, Client() will return an error.
func GetAILibrary(aiClient ai.Client) *object.Library {
	builder := object.NewLibraryBuilder("knot.ai", "Knot AI client library")

	// Client() returns the pre-configured AI client
	builder.FunctionWithHelp("Client", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		if err := errors.ExactArgs(args, 0); err != nil {
			return err
		}
		if aiClient == nil {
			return errors.NewError("AI client not available - no user context")
		}

		return scriptlingai.WrapClient(aiClient)
	}, fmt.Sprintf(`Client() - Get a pre-configured AI client instance.

Returns a client connected to the AI provider with MCP tools available.
Per-user tools are automatically discovered and executed.

Returns:
  Client: A pre-configured AI client instance.

Example:
  import knot.ai as ai
  import scriptling.ai.agent as agent

  client = ai.Client()
  bot = agent.Agent(client=client, system_prompt="You are helpful.")
  response = bot.trigger("Hello!", max_iterations=5)
  print(response.content)`))

	// get_default_model() returns the server-configured default model name
	builder.FunctionWithHelp("get_default_model", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return &object.String{Value: defaultModel}
	}, `get_default_model() - Get the server-configured default model name.

Returns:
  str: The model name configured on the server (e.g. "gpt-4o"), or empty string if not configured.

Example:
  model = knot.ai.get_default_model()
  print(f"Using model: {model}")`)

	return builder.Build()
}
