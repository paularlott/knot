package agentlink

import knotscriptling "github.com/paularlott/knot/internal/scriptling"

// Scripts in a space read personal secrets through the space's agent, the
// same path as `knot secret get`.
func init() {
	knotscriptling.UserSecretResolver = ResolveUserSecret
}
