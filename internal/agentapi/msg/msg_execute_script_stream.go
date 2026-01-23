package msg

type ExecuteScriptStreamMessage struct {
	Content   string   `msgpack:"content"`
	Arguments []string `msgpack:"arguments"`
	Timeout   int      `msgpack:"timeout"`
}
