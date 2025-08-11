package agentlink

type ConnectResponse struct {
	Success bool   `msgpack:"s"`
	Server  string `msgpack:"sv"`
	Token   string `msgpack:"t"`
}

type SpaceNoteRequest struct {
	Note string `json:"note" msgpack:"note"`
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
