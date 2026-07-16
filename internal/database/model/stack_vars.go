package model

// stackResolver builds the .stack.* template variable map for a given space —
// i.e. cross-space references to the space's siblings within its stack. It is
// injected by the service layer at server startup (see SetStackResolver) to
// avoid an import cycle between the model and database packages: model cannot
// import database, but the resolver needs DB access to look up sibling spaces.
//
// When the resolver is nil (e.g. during spec validation, on nodes where it is
// not registered, or when the space is not part of a stack) .stack is simply
// absent from the template data and any ${{ .stack.* }} reference renders as
// empty, the same fallback as any other missing variable.
var stackResolver func(space *Space, variables map[string]interface{}) map[string]interface{}

// SetStackResolver registers the function used to resolve .stack.* template
// variables. Intended to be called once at server startup.
func SetStackResolver(f func(space *Space, variables map[string]interface{}) map[string]interface{}) {
	stackResolver = f
}
