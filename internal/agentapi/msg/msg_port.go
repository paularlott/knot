package msg

type TcpPort struct {
	Port uint16
}

type HttpPort struct {
	Port       uint16
	ServerName string
}
