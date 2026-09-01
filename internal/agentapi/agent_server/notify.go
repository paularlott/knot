package agent_server

import (
	"github.com/paularlott/knot/internal/agentapi/msg"
)

// NotifyScriptLibsChanged tells agents that a `lib`-type script changed on the
// server so they drop their cached libs.zip package. A non-empty userId
// (a user library change) targets that user's space agents; an empty userId
// (a global library change) broadcasts to every connected agent.
func NotifyScriptLibsChanged(userId string) {
	sessionMutex.RLock()
	targets := make([]*Session, 0, len(sessions))
	for _, session := range sessions {
		if userId == "" || session.UserId == userId {
			targets = append(targets, session)
		}
	}
	sessionMutex.RUnlock()

	for _, session := range targets {
		if err := session.SendScriptLibsChanged(); err != nil {
			session.logger.WithError(err).Warn("sending script libs changed notification")
		}
	}
}

// SendScriptLibsChanged sends the fire-and-forget libs-changed notification to
// the agent on a new mux stream.
func (s *Session) SendScriptLibsChanged() error {
	conn, err := s.MuxSession.Open()
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := msg.WriteCommand(conn, msg.CmdScriptLibsChanged); err != nil {
		return err
	}
	return msg.WriteMessage(conn, &msg.ScriptLibsChanged{})
}
