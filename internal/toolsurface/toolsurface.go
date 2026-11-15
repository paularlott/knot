// Package toolsurface records which surface a tool call came from, so a tool
// can tell whether a person approved it. It sits below every package that
// makes in-process API calls (internal/util/rest among them), so those calls
// can strip the mark: an approval belongs to the request that obtained it and
// must not reach nested calls made on its behalf.
package toolsurface

import "context"

// Surface names where a tool call came from. The web assistant asks the user
// before tool calls and shows their arguments; an MCP client is expected to
// do the same for a tool marked destructive. Everything else sets no
// surface.
type Surface string

const (
	WebChat Surface = "web-chat"
	MCP     Surface = "mcp"
)

type key struct{}

// With marks ctx as coming from a surface that approves tool calls.
func With(ctx context.Context, surface Surface) context.Context {
	return context.WithValue(ctx, key{}, surface)
}

// From returns the surface ctx was marked with, "" when none.
func From(ctx context.Context) Surface {
	if ctx == nil {
		return ""
	}
	s, _ := ctx.Value(key{}).(Surface)
	return s
}

// Strip returns ctx without a surface mark, for a nested call (an in-process
// API request, a plugin tool, a chat completion) that must not inherit the
// approval of the request it runs under.
func Strip(ctx context.Context) context.Context {
	if From(ctx) == "" {
		return ctx
	}
	return context.WithValue(ctx, key{}, Surface(""))
}
