package scriptling

import (
	"context"
	"fmt"

	ai "github.com/paularlott/mcp/ai"
	"github.com/paularlott/scriptling/errors"
	scriptlingai "github.com/paularlott/scriptling/extlibs/ai"
	"github.com/paularlott/scriptling/object"
)

// GetAILibrary returns the knot.ai library for scriptling environments.
// Only Client() and get_default_model() are exposed, matching the Python standalone interface.
// When aiClient is nil, Client() will return an error.
func GetAILibrary(aiClient ai.Client) *object.Library {
	builder := object.NewLibraryBuilder("knot.ai", "Knot AI client library")

	builder.FunctionWithHelp("get_default_model", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return &object.String{Value: ""}
	}, `get_default_model() - Returns "" in embedded contexts; the server uses its configured default model when "" is passed to completion().`)

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

	return builder.Build()
}
