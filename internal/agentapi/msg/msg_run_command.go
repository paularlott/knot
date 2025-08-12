package msg

type RunCommandMessage struct {
	Command string   `msgpack:"command"`
	Args    []string `msgpack:"args"`
	Timeout int      `msgpack:"timeout"`
	Workdir string   `msgpack:"workdir"`
}

type RunCommandResponse struct {
	Success bool   `msgpack:"success"`
	Error   string `msgpack:"error"`
	Output  []byte `msgpack:"output"`
}
