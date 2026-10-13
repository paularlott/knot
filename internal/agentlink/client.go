package agentlink

import (
	"errors"
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

// ResolveUserSecret reads one of the space owner's personal secrets through
// the agent running in this space. It works only inside a space.
func ResolveUserSecret(path, field string) (string, error) {
	if !IsAgentRunning() {
		return "", fmt.Errorf("personal secrets can only be read inside a knot space")
	}

	var response UserSecretResponse
	if err := SendWithResponseMsg(CommandResolveUserSecret, &UserSecretRequest{Path: path, Field: field}, &response); err != nil {
		return "", fmt.Errorf("failed to ask the space agent: %w", err)
	}
	if response.Error != "" {
		return "", errors.New(response.Error)
	}
	return response.Value, nil
}
