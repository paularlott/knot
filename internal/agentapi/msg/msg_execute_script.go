package msg

type ExecuteScriptMessage struct {
	Content      string   `msgpack:"content"`
	Arguments    []string `msgpack:"arguments"`
	Timeout      int      `msgpack:"timeout"`
	IsSystemCall bool     `msgpack:"is_system_call"`
}

type ExecuteScriptResponse struct {
	Success bool   `msgpack:"success"`
	Error   string `msgpack:"error"`
	Output  string `msgpack:"output"`
}
