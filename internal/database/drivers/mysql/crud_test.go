package driver_mysql

import (
	"testing"
	"time"
)

func TestFormatDatabaseTimeUsesUTC(t *testing.T) {
	local := time.Date(2026, 5, 22, 22, 29, 56, 0, time.FixedZone("AWST", 8*60*60))

	formatted := formatDatabaseTime(&local, "2006-01-02 15:04:05")
	if formatted != "2026-05-22 14:29:56" {
		t.Fatalf("expected UTC-formatted time, got %q", formatted)
	}
}
