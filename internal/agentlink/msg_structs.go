package agentlink

import "github.com/paularlott/knot/internal/methods"

type ConnectResponse struct {
	Success bool   `msgpack:"s"`
	Server  string `msgpack:"sv"`
	Token   string `msgpack:"t"`
	SpaceID string `msgpack:"sid"`
}

type SpaceNoteRequest struct {
	Note string `json:"note" msgpack:"note"`
}

type SpaceFieldRequest struct {
	Name  string `json:"name" msgpack:"name"`
	Value string `json:"value" msgpack:"value"`
}

type SpaceGetFieldRequest struct {
	Name string `json:"name" msgpack:"name"`
}

type SpaceGetFieldResponse struct {
	Value string `json:"value" msgpack:"value"`
}

type RunCommandRequest struct {
	Command string `json:"command" msgpack:"command"`
	Timeout int    `json:"timeout" msgpack:"timeout"`
	Workdir string `json:"workdir" msgpack:"workdir"`
}

type RunCommandResponse struct {
	Success bool   `json:"success" msgpack:"success"`
	Error   string `json:"error" msgpack:"error"`
}

type ForwardPortRequest struct {
	LocalPort  uint16 `json:"local_port" msgpack:"local_port"`
	Space      string `json:"space" msgpack:"space"`
	RemotePort uint16 `json:"remote_port" msgpack:"remote_port"`
	Persistent bool   `json:"persistent" msgpack:"persistent"`
	Force      bool   `json:"force" msgpack:"force"`
}

type PortForwardInfo struct {
	LocalPort  uint16 `json:"local_port" msgpack:"local_port"`
	Space      string `json:"space" msgpack:"space"`
	RemotePort uint16 `json:"remote_port" msgpack:"remote_port"`
	Persistent bool   `json:"persistent" msgpack:"persistent"`
}

type ListPortForwardsResponse struct {
	Forwards []PortForwardInfo `json:"forwards" msgpack:"forwards"`
}

type StopPortForwardRequest struct {
	LocalPort uint16 `json:"local_port" msgpack:"local_port"`
}

type RegisterMethodsRequest struct {
	Registration methods.Registration `json:"registration" msgpack:"registration"`
}

type RegisterMethodsResponse struct {
	Success bool   `json:"success" msgpack:"success"`
	Error   string `json:"error,omitempty" msgpack:"error,omitempty"`
}

// RegisterMethodsFileRequest ships a raw file from the CLI to the agent daemon
// for processing. Content is the file body. Args is optional and only used by
// the script variant (passed in as sys.argv).
type RegisterMethodsFileRequest struct {
	Content string   `json:"content" msgpack:"content"`
	Args    []string `json:"args,omitempty" msgpack:"args,omitempty"`
}
