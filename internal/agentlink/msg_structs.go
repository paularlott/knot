package agentlink

type ConnectResponse struct {
	Success bool   `msgpack:"s"`
	Server  string `msgpack:"sv"`
	Token   string `msgpack:"t"`
}

type SpaceNoteRequest struct {
	Note string `json:"note" msgpack:"note"`
}
