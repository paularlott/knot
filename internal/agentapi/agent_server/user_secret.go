package agent_server

import (
	"net"

	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/log"
)

// Personal secrets (Knot Pro). The request message is defined in both
// editions so agents and servers stay protocol-compatible; resolving exists
// only in Pro, which installs this hook. In Core every request is answered
// with an error saying the feature needs Knot Pro.

// UserSecretResolveHook resolves a personal secret for the space behind the
// session: the space owner's token, permission and the space's template
// all apply inside the implementation.
var UserSecretResolveHook func(session *Session, path, field string) (string, error)

func handleResolveUserSecret(stream net.Conn, session *Session) {
	var request msg.UserSecretRequest
	if err := msg.ReadMessage(stream, &request); err != nil {
		log.WithError(err).Error("reading user secret request:")
		return
	}

	response := msg.UserSecretResponse{}
	if UserSecretResolveHook == nil {
		response.Error = "personal secrets are not supported by this server (requires Knot Pro)"
	} else if value, err := UserSecretResolveHook(session, request.Path, request.Field); err != nil {
		response.Error = err.Error()
	} else {
		response.Value = value
	}

	if err := msg.WriteMessage(stream, &response); err != nil {
		log.WithError(err).Error("writing user secret response:")
	}
}
