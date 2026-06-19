package agentlink

import (
	"net"

	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/methods"
)

// handleRegisterMethodsTOML receives raw TOML bytes from the CLI, decodes them
// in the agent daemon, and forwards the resulting Registration to the knot
// server via agentClient.RegisterMethods (which also starts the method server).
func handleRegisterMethodsTOML(conn net.Conn, msg *CommandMsg) {
	var req RegisterMethodsFileRequest
	if err := msg.Unmarshal(&req); err != nil {
		log.WithError(err).Error("failed to unmarshal register methods TOML request")
		_ = sendMsg(conn, CommandRegisterMethodsTOML, RegisterMethodsResponse{Success: false, Error: err.Error()})
		return
	}

	if agentClient == nil {
		_ = sendMsg(conn, CommandRegisterMethodsTOML, RegisterMethodsResponse{Success: false, Error: "agent is not connected"})
		return
	}

	reg, err := methods.LoadRawTOML([]byte(req.Content))
	if err != nil {
		_ = sendMsg(conn, CommandRegisterMethodsTOML, RegisterMethodsResponse{Success: false, Error: err.Error()})
		return
	}

	if err := agentClient.RegisterMethods(reg); err != nil {
		_ = sendMsg(conn, CommandRegisterMethodsTOML, RegisterMethodsResponse{Success: false, Error: err.Error()})
		return
	}

	_ = sendMsg(conn, CommandRegisterMethodsTOML, RegisterMethodsResponse{Success: true})
}

// handleRegisterMethodsScript receives a Scriptling script's source from the
// CLI and runs it directly in the agent daemon. The daemon-side script runner
// wires knot.methods.SetMethodsRegistrar to agentClient.RegisterMethods so
// server.register() inside the script starts the method server and publishes
// the registration to the knot server without any further IPC.
func handleRegisterMethodsScript(conn net.Conn, msg *CommandMsg) {
	var req RegisterMethodsFileRequest
	if err := msg.Unmarshal(&req); err != nil {
		log.WithError(err).Error("failed to unmarshal register methods script request")
		_ = sendMsg(conn, CommandRegisterMethodsScript, RegisterMethodsResponse{Success: false, Error: err.Error()})
		return
	}

	if methodsScriptRunner == nil {
		_ = sendMsg(conn, CommandRegisterMethodsScript, RegisterMethodsResponse{Success: false, Error: "agent is not connected"})
		return
	}

	if err := methodsScriptRunner(req.Content, req.Args); err != nil {
		_ = sendMsg(conn, CommandRegisterMethodsScript, RegisterMethodsResponse{Success: false, Error: err.Error()})
		return
	}

	_ = sendMsg(conn, CommandRegisterMethodsScript, RegisterMethodsResponse{Success: true})
}
