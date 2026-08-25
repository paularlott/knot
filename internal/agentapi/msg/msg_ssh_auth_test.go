package msg

import (
	"net"
	"testing"
	"time"
)

func TestSSHAuthResultRoundTrip(t *testing.T) {
	when := time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)
	original := &SSHAuthResultMessage{
		Success:        false,
		KeyFingerprint: "SHA256:abc123",
		RemoteAddr:     "203.0.113.9:51422",
		When:           when,
	}

	client, serverConn := net.Pipe()
	defer client.Close()
	defer serverConn.Close()

	go func() {
		_ = SendSSHAuthResult(client, original)
	}()

	cmd, err := ReadCommand(serverConn)
	if err != nil {
		t.Fatal(err)
	}
	if cmd != byte(CmdSSHPortAuthResult) {
		t.Fatalf("command = %d, want %d", cmd, CmdSSHPortAuthResult)
	}

	var decoded SSHAuthResultMessage
	if err := ReadMessage(serverConn, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Success || decoded.KeyFingerprint != "SHA256:abc123" || decoded.RemoteAddr != "203.0.113.9:51422" {
		t.Errorf("round trip lost fields: %+v", decoded)
	}
	if !decoded.When.Equal(when) {
		t.Errorf("When not preserved: %v", decoded.When)
	}
}
