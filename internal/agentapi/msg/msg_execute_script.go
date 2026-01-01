package msg

type ExecuteScriptMessage struct {
	Content      string            `msgpack:"content"`
	Libraries    map[string]string `msgpack:"libraries"`
	Arguments    []string          `msgpack:"arguments"`
	Timeout      int               `msgpack:"timeout"`
	IsSystemCall bool              `msgpack:"is_system_call"` // true for startup/shutdown scripts
}

type ExecuteScriptResponse struct {
	Success bool   `msgpack:"success"`
	Error   string `msgpack:"error"`
	Output  string `msgpack:"output"`
}
