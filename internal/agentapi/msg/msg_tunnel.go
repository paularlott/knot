package msg

import "net"

// Web tunnel management (server -> agent). These drive the agent's
// agent-owned tunnel registry (the same one the in-space agentlink path uses),
// letting a desktop client manage a space's web tunnels remotely.

type TunnelStartRequest struct {
	Protocol      string `json:"protocol" msgpack:"protocol"`
	Port          uint16 `json:"port" msgpack:"port"`
	Name          string `json:"name" msgpack:"name"`
	TlsName       string `json:"tls_name,omitempty" msgpack:"tls_name,omitempty"`
	TlsSkipVerify bool   `json:"tls_skip_verify,omitempty" msgpack:"tls_skip_verify,omitempty"`
}

type TunnelStartResponse struct {
	Success bool   `json:"success" msgpack:"success"`
	Error   string `json:"error,omitempty" msgpack:"error,omitempty"`
	URL     string `json:"url,omitempty" msgpack:"url,omitempty"`
}

type TunnelInfo struct {
	Port     uint16 `json:"port" msgpack:"port"`
	Protocol string `json:"protocol" msgpack:"protocol"`
	Name     string `json:"name" msgpack:"name"`
	URL      string `json:"url" msgpack:"url"`
}

type TunnelListResponse struct {
	Tunnels []TunnelInfo `json:"tunnels" msgpack:"tunnels"`
}

type TunnelStopRequest struct {
	Name string `json:"name" msgpack:"name"`
}

type TunnelStopResponse struct {
	Success bool   `json:"success" msgpack:"success"`
	Error   string `json:"error,omitempty" msgpack:"error,omitempty"`
}

func SendTunnelStart(conn net.Conn, req *TunnelStartRequest) (*TunnelStartResponse, error) {
	if err := WriteCommand(conn, CmdTunnelStart); err != nil {
		return nil, err
	}
	if err := WriteMessage(conn, req); err != nil {
		return nil, err
	}
	var resp TunnelStartResponse
	if err := ReadMessage(conn, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
