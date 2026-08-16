package agent_server

import (
	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/database/model"
)

// Log sink support (Knot Pro). The wire protocol — register fields, the
// CmdMirrorLog command and the mirror message — is defined in both editions
// so agents and servers stay protocol-compatible, but the ability for a
// space to become a log sink exists only in Pro: these hooks are nil in
// Core and installed by Pro's agent_server package (licence- and
// permission-gated). Guard every call with a nil check.

// LogSinkRegisterHook is called when an agent advertises a log sink in its
// registration message. Receives the session and the (already loaded) owner.
var LogSinkRegisterHook func(session *Session, user *model.User, port int, format string)

// LogSinkMirrorHook is called (leader-gated) for each log record received
// from a space, with the space id, its owner's user id and username, and the
// space name — owner scoping is enforced inside the implementation.
var LogSinkMirrorHook func(spaceId, userId, username, spaceName string, logMsg *msg.LogMessage)

// LogSinkUnregisterHook is called when a space's session is removed, so a
// registered sink stops receiving mirrors.
var LogSinkUnregisterHook func(spaceId string)
