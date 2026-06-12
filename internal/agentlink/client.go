package agentlink

import (
	"fmt"
)

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

func GetSpaceId() (string, error) {
	if !IsAgentRunning() {
		return "", fmt.Errorf("agent not running")
	}

	var response ConnectResponse
	err := SendWithResponseMsg(CommandConnect, nil, &response)
	if err != nil {
		return "", fmt.Errorf("failed to get space ID: %w", err)
	}

	if !response.Success {
		return "", fmt.Errorf("agent returned unsuccessful response")
	}

	return response.SpaceID, nil
}
