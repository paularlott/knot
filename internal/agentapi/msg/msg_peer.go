package msg

import "net"

// PeerIntroduce is sent from the server to an agent to tell it the address of
// another agent it can connect to directly. The agent should attempt a direct
// connection to the given address; if it fails, it falls back to relay via the
// server.
type PeerIntroduce struct {
	TargetSpace   string `msgpack:"target_space"`    // space name (matches PortForwardRequest.Space)
	TargetSpaceId string `msgpack:"target_space_id"` // space ID (for logging)
	Host          string `msgpack:"host"`
	Port          uint16 `msgpack:"port"`
}

// PeerEndpoint is a dialable address for a peer agent's direct listener.
type PeerEndpoint struct {
	Host string
	Port uint16
}

// PeerConnect is the first message a dialing agent sends on a direct
// peer-to-peer connection. It identifies who is calling.
type PeerConnect struct {
	SourceSpaceId string `msgpack:"source_space_id"`
}

// PeerChallenge is sent by the listening agent. It contains a random nonce
// that the dialer must HMAC with the shared zone agentToken.
type PeerChallenge struct {
	Nonce []byte `msgpack:"nonce"`
}

// PeerAuth is the dialer's response to a PeerChallenge: HMAC-SHA256 of the
// nonce keyed by the zone-wide agentToken.
type PeerAuth struct {
	Response []byte `msgpack:"response"`
}

// PeerAuthResult tells the dialer whether authentication succeeded.
type PeerAuthResult struct {
	Success bool   `msgpack:"success"`
	Error   string `msgpack:"error,omitempty"`
}

// SendPeerIntroduce writes a PeerIntroduce message on the given stream.
func SendPeerIntroduce(conn net.Conn, intro *PeerIntroduce) error {
	if err := WriteCommand(conn, CmdPeerIntroduce); err != nil {
		return err
	}
	return WriteMessage(conn, intro)
}

// PeerRequestIntro is sent from an agent to the server when a direct connection
// fails and the agent suspects the stored endpoint is stale (e.g. the target
// space restarted with a new port). The server resolves the target's current
// address and responds with a PeerIntroduce.
type PeerRequestIntro struct {
	Space string `msgpack:"space"`
}
