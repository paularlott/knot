package agenttunnel

import "fmt"

// Credentials resolves the server, token and TLS verification setting for a
// tunnel request that may carry its own target: a complete server/token pair
// targets that knot server (skipVerify, the requester's choice for it), a
// partial pair is an error, and otherwise the agent's own server is used.
//
// Shared by the agentlink (in-space CLI) and server-relay (remote desktop)
// tunnel start paths so both resolve targets identically.
func Credentials(server, token string, skipVerify bool, agentServer, agentToken string, agentSkipVerify bool) (string, string, bool, error) {
	switch {
	case server != "" && token != "":
		return server, token, skipVerify, nil

	case server != "" || token != "":
		return "", "", false, fmt.Errorf("both server and token are required to target another server")

	case agentServer == "" || agentToken == "":
		return "", "", false, fmt.Errorf("failed to get connection info from agent")
	}

	return agentServer, agentToken, agentSkipVerify, nil
}
