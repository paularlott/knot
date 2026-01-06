package msg

type TcpPort struct {
	Port uint16
}

type HttpPort struct {
	Port       uint16
	ServerName string
}

// Port forward between spaces
type PortForwardRequest struct {
	LocalPort  uint16 `json:"local_port" msgpack:"local_port"`
	Space      string `json:"space" msgpack:"space"`
	RemotePort uint16 `json:"remote_port" msgpack:"remote_port"`
}

type PortForwardResponse struct {
	Success bool   `json:"success" msgpack:"success"`
	Error   string `json:"error" msgpack:"error"`
}

type PortListResponse struct {
	Forwards []PortForwardInfo `json:"forwards" msgpack:"forwards"`
}

type PortForwardInfo struct {
	LocalPort  uint16 `json:"local_port" msgpack:"local_port"`
	Space      string `json:"space" msgpack:"space"`
	RemotePort uint16 `json:"remote_port" msgpack:"remote_port"`
}

type PortStopRequest struct {
	LocalPort uint16 `json:"local_port" msgpack:"local_port"`
}

type PortStopResponse struct {
	Success bool   `json:"success" msgpack:"success"`
	Error   string `json:"error" msgpack:"error"`
}
