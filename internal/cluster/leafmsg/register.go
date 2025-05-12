package leafmsg

type Register struct {
	LeafVersion string `json:"leaf_version" msgpack:"leaf_version"`
	Location    string `json:"location" msgpack:"location"`
}

type RegisterResponse struct {
	Success bool   `json:"success" msgpack:"success"`
	Error   string `json:"error" msgpack:"error"`
}
