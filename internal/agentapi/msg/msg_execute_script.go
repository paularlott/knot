package msg

type ExecuteScriptMessage struct {
	Content   string            `msgpack:"content"`
	Libraries map[string]string `msgpack:"libraries"`
	Arguments []string          `msgpack:"arguments"`
	Timeout   int               `msgpack:"timeout"`
}

type ExecuteScriptResponse struct {
	Success bool   `msgpack:"success"`
	Error   string `msgpack:"error"`
	Output  string `msgpack:"output"`
}
