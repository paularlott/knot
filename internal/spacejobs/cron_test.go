package spacejobs

import (
	"testing"
	"time"
)

func at(t *testing.T, value string) time.Time {
	t.Helper()
	ts, err := time.ParseInLocation("2006-01-02 15:04", value, time.UTC)
	if err != nil {
		t.Fatalf("bad test time %q: %v", value, err)
	}
	return ts
}

func TestParseCronValid(t *testing.T) {
	tests := []struct {
		expr string
	}{
		{"* * * * *"},
		{"5 * * * *"},
		{"5 1 * * *"},
		{"*/5 * * * *"},
		{"0,30 * * * *"},
		{"1-5 * * * *"},
		{"1-30/2 * * * *"},
		{"30 9 * * 1-5"},
		{"0 0 1 1 *"},
		{"0 12 * * 0"},
		{"0 12 * * 7"}, // 7 wraps to Sunday
		{"59 23 31 12 *"},
	}
	for _, tc := range tests {
		if _, err := parseCron(tc.expr); err != nil {
			t.Errorf("parseCron(%q): unexpected error: %v", tc.expr, err)
		}
	}
}

func TestParseCronInvalid(t *testing.T) {
	tests := []string{
		"",
		"* * * *",      // too few fields
		"* * * * * *",  // too many fields
		"60 * * * *",   // minute out of range
		"* 24 * * *",   // hour out of range
		"* * 32 * *",   // day out of range
		"* * * 13 *",   // month out of range
		"* * * * 8",    // weekday out of range
		"*/0 * * * *",  // zero step
		"a * * * *",    // not a number
		"5-1 * * * *",  // inverted range
		"1,,2 * * * *", // empty list element
		"0 0 30 2 *",   // never matches a date
	}
	for _, expr := range tests {
		if _, err := parseCron(expr); err == nil {
			t.Errorf("parseCron(%q): expected error, got none", expr)
		}
	}
}

func TestCronMatches(t *testing.T) {
	tests := []struct {
		expr string
		time string
		want bool
	}{
		{"* * * * *", "2026-08-19 10:00", true},
		{"5 * * * *", "2026-08-19 10:05", true},
		{"5 * * * *", "2026-08-19 10:06", false},
		{"5 1 * * *", "2026-08-19 01:05", true},
		{"5 1 * * *", "2026-08-19 13:05", false},
		{"*/15 * * * *", "2026-08-19 10:45", true},
		{"*/15 * * * *", "2026-08-19 10:50", false},
		{"30 9 * * 1-5", "2026-08-19 09:30", true},  // Wednesday
		{"30 9 * * 1-5", "2026-08-22 09:30", false}, // Saturday
		{"0 0 1 1 *", "2026-01-01 00:00", true},
		{"0 0 1 1 *", "2026-02-01 00:00", false},
		{"0 12 * * 0", "2026-08-23 12:00", true}, // Sunday
		{"0 12 * * 7", "2026-08-23 12:00", true}, // 7 = Sunday
	}
	for _, tc := range tests {
		sched, err := parseCron(tc.expr)
		if err != nil {
			t.Fatalf("parseCron(%q): %v", tc.expr, err)
		}
		if got := sched.matches(at(t, tc.time)); got != tc.want {
			t.Errorf("matches(%q, %q) = %v, want %v", tc.expr, tc.time, got, tc.want)
		}
	}
}

func TestCronNext(t *testing.T) {
	tests := []struct {
		expr  string
		after string
		want  string
	}{
		{"* * * * *", "2026-08-19 10:00", "2026-08-19 10:01"},
		{"5 1 * * *", "2026-08-19 01:05", "2026-08-20 01:05"},
		{"30 9 * * 1-5", "2026-08-19 09:31", "2026-08-20 09:30"}, // Wed -> Thu
		{"30 9 * * 1-5", "2026-08-21 09:31", "2026-08-24 09:30"}, // Fri -> Mon
		{"0 0 29 2 *", "2026-08-19 10:00", "2028-02-29 00:00"},   // leap day
	}
	for _, tc := range tests {
		sched, err := parseCron(tc.expr)
		if err != nil {
			t.Fatalf("parseCron(%q): %v", tc.expr, err)
		}
		next, ok := sched.next(at(t, tc.after))
		if !ok {
			t.Fatalf("next(%q, %q): no match found", tc.expr, tc.after)
		}
		if got := next.Format("2006-01-02 15:04"); got != tc.want {
			t.Errorf("next(%q, %q) = %s, want %s", tc.expr, tc.after, got, tc.want)
		}
	}
}

func TestCronNextRespectsLocation(t *testing.T) {
	loc := time.FixedZone("test", 2*60*60) // UTC+2
	sched, err := parseCron("0 9 * * *")
	if err != nil {
		t.Fatalf("parseCron: %v", err)
	}
	after := time.Date(2026, 8, 19, 8, 0, 0, 0, loc)
	next, ok := sched.next(after)
	if !ok {
		t.Fatal("no next match")
	}
	if next.Hour() != 9 || next.Location() != loc {
		t.Errorf("next = %v, want 09:00 in original location", next)
	}
}

func TestCronDomDowSemantics(t *testing.T) {
	// When both day-of-month and day-of-week are restricted, standard cron
	// fires when either matches (vixie semantics).
	sched, err := parseCron("0 0 13 * 5") // 13th of the month OR Friday
	if err != nil {
		t.Fatalf("parseCron: %v", err)
	}
	if !sched.matches(at(t, "2026-08-14 00:00")) { // Friday the 14th
		t.Error("expected Friday to match")
	}
	if !sched.matches(at(t, "2026-08-13 00:00")) { // the 13th (Thursday)
		t.Error("expected 13th to match")
	}
	if sched.matches(at(t, "2026-08-18 00:00")) { // Tuesday the 18th
		t.Error("expected neither-day to not match")
	}
}
