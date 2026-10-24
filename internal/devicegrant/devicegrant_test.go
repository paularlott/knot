package devicegrant

import (
	"strings"
	"testing"
)

func TestNormUserCode(t *testing.T) {
	tests := []struct {
		typed, want string
	}{
		{"WDJB-MJHT", "WDJBMJHT"},
		{"wdjb mjht", "WDJBMJHT"},
		{"WDJBMJHT", "WDJBMJHT"},
		{"WDJB-MJH", ""},   // too short
		{"WDJB-MJHTB", ""}, // too long
		{"WAJB-MJHT", ""},  // vowel, never in a code
		{"WDJB-MJH1", ""},  // digit, never in a code
		{strings.Repeat("B", 40), ""},
	}
	for _, tt := range tests {
		if got := NormUserCode(tt.typed); got != tt.want {
			t.Errorf("NormUserCode(%q) = %q, want %q", tt.typed, got, tt.want)
		}
	}
}

func TestGrantLifecycle(t *testing.T) {
	g, err := New("knot client test")
	if err != nil {
		t.Fatal(err)
	}
	if NormUserCode(FormatUserCode(g.UserCode)) != g.UserCode {
		t.Fatalf("user code %q doesn't survive formatting", g.UserCode)
	}

	found, limited := ByUserCode("user-1", strings.ToLower(FormatUserCode(g.UserCode)))
	if limited || found == nil || found.DeviceCode != g.DeviceCode {
		t.Fatalf("ByUserCode = %v, %v; want the new grant", found, limited)
	}

	found.Status = StatusApproved
	found.TokenId = "tk_test"
	Apply(found)

	// Once decided the user code stops working.
	if again, _ := ByUserCode("user-1", g.UserCode); again != nil {
		t.Error("an approved grant's user code should no longer be found")
	}

	// Gossip arriving late must not move the status back.
	stale := *g
	Apply(&stale)
	polled, _ := ByDeviceCode(g.DeviceCode)
	if polled == nil || polled.Status != StatusApproved || polled.TokenId != "tk_test" {
		t.Errorf("after stale gossip, grant = %+v; want approved with its token", polled)
	}
}

func TestPollSlowDown(t *testing.T) {
	g, err := New("knot client test")
	if err != nil {
		t.Fatal(err)
	}
	if _, slow := ByDeviceCode(g.DeviceCode); slow {
		t.Error("first poll should not be told to slow down")
	}
	if _, slow := ByDeviceCode(g.DeviceCode); !slow {
		t.Error("an immediate second poll should be told to slow down")
	}
	if found, _ := ByDeviceCode("no-such-code"); found != nil {
		t.Error("an unknown device code should not be found")
	}
}

func TestGuessLimit(t *testing.T) {
	g, err := New("knot client test")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < maxGuesses; i++ {
		if found, limited := ByUserCode("guesser", "BBBB-BBBB"); found != nil || limited {
			t.Fatalf("guess %d: found %v, limited %v", i, found, limited)
		}
	}
	// Even the right code is refused once the limit is reached.
	if found, limited := ByUserCode("guesser", g.UserCode); found != nil || !limited {
		t.Errorf("after %d wrong guesses: found %v, limited %v; want refused", maxGuesses, found, limited)
	}
	// Other users are unaffected.
	if found, limited := ByUserCode("someone-else", g.UserCode); found == nil || limited {
		t.Error("another user's guesses should not limit this one")
	}
}
