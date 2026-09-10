package agentlink

import (
	"testing"

	"github.com/shamaton/msgpack/v3"
)

// TestStartTunnelRequestMsgpackRoundTrip guards the wire format: the
// Server/Token/ServerTlsSkipVerify fields must survive a marshal/unmarshal
// cycle, and a request without them must decode with them empty.
func TestStartTunnelRequestMsgpackRoundTrip(t *testing.T) {
	full := StartTunnelRequest{
		Protocol:            "http",
		Port:                8080,
		Name:                "test1",
		TlsName:             "local",
		TlsSkipVerify:       true,
		Server:              "https://other.example.com",
		Token:               "tok-other",
		ServerTlsSkipVerify: true,
	}

	encoded, err := msgpack.Marshal(&full)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded StartTunnelRequest
	if err := msgpack.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded != full {
		t.Fatalf("round trip mismatch:\n got %+v\nwant %+v", decoded, full)
	}

	// A false ServerTlsSkipVerify must not be lost to omitempty — it is
	// always encoded.
	verify := full
	verify.ServerTlsSkipVerify = false
	encoded, err = msgpack.Marshal(&verify)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	decoded = StartTunnelRequest{}
	if err := msgpack.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.ServerTlsSkipVerify {
		t.Fatal("explicit --tls-skip-verify=false was lost in transit")
	}

	legacy := StartTunnelRequest{Protocol: "http", Port: 8080, Name: "test1"}
	encoded, err = msgpack.Marshal(&legacy)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	decoded = StartTunnelRequest{}
	if err := msgpack.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Server != "" || decoded.Token != "" {
		t.Fatalf("request without a target decoded with server=%q token=%q", decoded.Server, decoded.Token)
	}
}
