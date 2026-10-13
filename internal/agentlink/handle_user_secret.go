package agentlink

import (
	"net"

	"github.com/paularlott/knot/internal/log"
)

// handleResolveUserSecret reads a personal secret for a process in the
// space (the knot CLI or a script), asking the server over the agent link.
// It always answers, carrying the failure in Error, so callers see why.
func handleResolveUserSecret(conn net.Conn, msg *CommandMsg) error {
	var request UserSecretRequest
	if err := msg.Unmarshal(&request); err != nil {
		log.WithError(err).Error("Failed to unmarshal user secret request")
		return err
	}

	response := UserSecretResponse{}
	if value, err := agentClient.ResolveUserSecret(request.Path, request.Field); err != nil {
		response.Error = err.Error()
	} else {
		response.Value = value
	}

	if err := sendMsg(conn, CommandNil, &response); err != nil {
		log.WithError(err).Error("Failed to send response")
		return err
	}
	return nil
}
