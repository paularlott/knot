package leafmsg

type Register struct {
	LeafVersion string `json:"leaf_version" msgpack:"leaf_version"`
	Zone        string `json:"zone" msgpack:"zone"`
}

type RegisterResponse struct {
	Success bool   `json:"success" msgpack:"success"`
	Error   string `json:"error" msgpack:"error"`
}
