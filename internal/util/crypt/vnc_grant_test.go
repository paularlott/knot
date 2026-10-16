package crypt

import (
	"strings"
	"testing"
)

func TestVNCGrant(t *testing.T) {
	key := CreateKey()
	host := "alice--box--vnc.knot.example.com"
	sealed, err := SealVNCGrant(key, host, VNCGrant{Kind: VNCGrantCookie, SessionId: "session-1"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sealed, "session-1") {
		t.Fatal("grant shows the session ID")
	}

	g, ok := OpenVNCGrant(key, host, sealed)
	if !ok || g.Kind != VNCGrantCookie || g.SessionId != "session-1" || g.Nonce == "" {
		t.Fatalf("open = %+v, %v", g, ok)
	}
	// Host names are case-insensitive.
	if _, ok := OpenVNCGrant(key, strings.ToUpper(host), sealed); !ok {
		t.Error("didn't open for the same host in upper case")
	}

	for name, open := range map[string]func() bool{
		"another host": func() bool { _, ok := OpenVNCGrant(key, "bob--box--vnc.knot.example.com", sealed); return ok },
		"another key":  func() bool { _, ok := OpenVNCGrant(CreateKey(), host, sealed); return ok },
		"tampered":     func() bool { _, ok := OpenVNCGrant(key, host, sealed[:len(sealed)-2]+"AA"); return ok },
		"junk":         func() bool { _, ok := OpenVNCGrant(key, host, "junk"); return ok },
	} {
		if open() {
			t.Errorf("opened with %s", name)
		}
	}
}
