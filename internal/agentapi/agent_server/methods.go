package agent_server

import (
	"fmt"
	"net"
	"time"

	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/methods"
)
func handleRegisterMethods(stream net.Conn, session *Session) {
	var req msg.RegisterMethodsRequest
	if err := msg.ReadMessage(stream, &req); err != nil {
		log.WithError(err).Error("reading register methods message")
		return
	}

	db := database.GetInstance()
	space, err := db.GetSpace(session.Id)
	if err != nil {
		log.Warn("handleRegisterMethods: unknown space", "space_id", session.Id)
		_ = msg.WriteMessage(stream, &msg.RegisterMethodsResponse{Success: false, Error: "unknown space"})
		return
	}
	owner, err := db.GetUser(space.UserId)
	if err != nil {
		log.Warn("handleRegisterMethods: unknown space owner", "space_id", session.Id, "owner_id", space.UserId)
		_ = msg.WriteMessage(stream, &msg.RegisterMethodsResponse{Success: false, Error: "unknown space owner"})
		return
	}

	if err := methods.NormalizeAndValidate(&req.Registration, space.Name); err != nil {
		log.Warn("handleRegisterMethods: validation failed", "space", space.Name, "error", err.Error())
		_ = msg.WriteMessage(stream, &msg.RegisterMethodsResponse{Success: false, Error: err.Error()})
		return
	}
	if err := resolveMethodGroups(database.GetInstance(), &req.Registration); err != nil {
		log.Warn("handleRegisterMethods: group resolution failed", "space", space.Name, "error", err.Error())
		_ = msg.WriteMessage(stream, &msg.RegisterMethodsResponse{Success: false, Error: err.Error()})
		return
	}
	if err := methods.DefaultRegistry().Register(space, owner, &req.Registration); err != nil {
		log.Warn("handleRegisterMethods: registry rejected registration", "space", space.Name, "error", err.Error())
		_ = msg.WriteMessage(stream, &msg.RegisterMethodsResponse{Success: false, Error: err.Error()})
		return
	}

	names := make([]string, 0, len(req.Registration.Methods))
	for _, m := range req.Registration.Methods {
		names = append(names, m.Name)
	}
	log.Debug("methods registered",
		"space", space.Name,
		"space_id", space.Id,
		"owner", owner.Username,
		"owner_id", owner.Id,
		"method_count", len(req.Registration.Methods),
		"methods", names,
	)

	_ = msg.WriteMessage(stream, &msg.RegisterMethodsResponse{Success: true})
}

// handleUnregisterMethods removes all methods registered by the space. Called
// when the agent's stdio method server process exits (the methods are no
// longer callable). The session itself stays alive — only the method
// registrations are removed from the registry.
func handleUnregisterMethods(stream net.Conn, session *Session) {
	methods.DefaultRegistry().UnregisterSpace(session.Id)
	log.Info("methods unregistered", "space_id", session.Id)
}

func (s *Session) SendCallMethod(call *msg.CallMethodRequest, timeoutSeconds int) (*msg.CallMethodResponse, error) {
	conn, err := s.MuxSession.Open()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if err := msg.WriteCommand(conn, msg.CmdCallMethod); err != nil {
		return nil, err
	}
	if err := msg.WriteMessage(conn, call); err != nil {
		return nil, err
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 30
	}
	var response msg.CallMethodResponse
	if err := msg.ReadMessageWithTimeout(conn, &response, time.Duration(timeoutSeconds)*time.Second); err != nil {
		return nil, err
	}
	return &response, nil
}

// resolveMethodGroups rewrites each method's Groups slice in place so that it
// contains group IDs (which is what user.Groups holds and what HasAnyGroup
// compares against). Entries are matched as either a group name or an
// existing group ID — both forms are accepted. Unknown groups fail
// registration so a typo doesn't silently exclude every caller.
//
// Lives here (rather than in the methods package) because it needs database
// access; the methods package is deliberately DB-free.
func resolveMethodGroups(db database.DbDriver, reg *methods.Registration) error {
	// Fast path: nothing to resolve.
	hasGroups := false
	for _, m := range reg.Methods {
		if len(m.Groups) > 0 {
			hasGroups = true
			break
		}
	}
	if !hasGroups {
		return nil
	}

	groups, err := db.GetGroups()
	if err != nil {
		return err
	}
	byName := make(map[string]string, len(groups))
	byID := make(map[string]bool, len(groups))
	for _, g := range groups {
		byName[g.Name] = g.Id
		byID[g.Id] = true
	}

	for i := range reg.Methods {
		method := &reg.Methods[i]
		if len(method.Groups) == 0 {
			continue
		}
		resolved := make([]string, 0, len(method.Groups))
		for _, in := range method.Groups {
			if in == "" {
				continue
			}
			// Accept existing IDs as-is so callers that already use IDs
			// (e.g. via the web UI's convention) keep working.
			if byID[in] {
				resolved = append(resolved, in)
				continue
			}
			if id, ok := byName[in]; ok {
				resolved = append(resolved, id)
				continue
			}
			return fmt.Errorf("unknown group: %s", in)
		}
		method.Groups = resolved
	}
	return nil
}
