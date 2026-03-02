package agentlink

import (
	"fmt"
)

// GetConnectionInfo retrieves connection information from the running agent
// Returns server URL and auth token
func GetConnectionInfo() (server, token string, err error) {
	if !IsAgentRunning() {
		return "", "", fmt.Errorf("agent not running")
	}

	var response ConnectResponse
	err = SendWithResponseMsg(CommandConnect, nil, &response)
	if err != nil {
		return "", "", fmt.Errorf("failed to get connection info: %w", err)
	}

	if !response.Success {
		return "", "", fmt.Errorf("agent returned unsuccessful response")
	}

	return response.Server, response.Token, nil
}
