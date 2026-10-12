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
	Output  []byte `msgpack:"output"` // stdout and stderr, interleaved
	// The two streams apart, so the CLI can keep stderr out of redirected
	// stdout. Empty from agents that predate them.
	Stdout []byte `msgpack:"stdout"`
	Stderr []byte `msgpack:"stderr"`
	// ExitCode is the command's exit status: 0 on success, 124 when it
	// timed out, 1 when it couldn't be run.
	ExitCode int `msgpack:"exit_code"`
}
