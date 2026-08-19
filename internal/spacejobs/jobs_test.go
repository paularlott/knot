package spacejobs

import (
	"testing"
)

func TestParseJobsFile(t *testing.T) {
	config, err := parseJobsFile([]byte(`
[jobs.backup]
command = "./backup.sh"
hour = 2
minute = 0

[jobs.build]
command = "make nightly"
schedule = "30 9 * * 1-5"
enabled = false

[jobs.cleanup]
command = "./clean.sh"

[jobs.pulse]
command = "./pulse.sh"
minute = "*/5"

[jobs.broken]
command = "./broken.sh"
minute = "not-a-number"

[jobs.both]
command = "true"
schedule = "* * * * *"
minute = 5

[jobs.nocommand]
minute = 30
`))
	if err != nil {
		t.Fatalf("parseJobsFile: %v", err)
	}

	byName := map[string]*Job{}
	for _, job := range config.jobs {
		byName[job.Name] = job
	}

	if len(byName) != 4 {
		t.Fatalf("expected 4 valid jobs, got %d: %v", len(byName), byName)
	}

	backup := byName["backup"]
	if backup.Cron != "0 2 * * *" {
		t.Errorf("backup.Cron = %q, want %q", backup.Cron, "0 2 * * *")
	}
	if !backup.EnabledDefault {
		t.Error("backup should default to enabled")
	}

	build := byName["build"]
	if build.Cron != "30 9 * * 1-5" {
		t.Errorf("build.Cron = %q, want %q", build.Cron, "30 9 * * 1-5")
	}
	if build.EnabledDefault {
		t.Error("build should be disabled by authored default")
	}

	cleanup := byName["cleanup"]
	if !cleanup.ManualOnly || cleanup.Cron != "" {
		t.Errorf("cleanup should be manual-only, got cron %q", cleanup.Cron)
	}

	pulse := byName["pulse"]
	if pulse.Cron != "*/5 * * * *" {
		t.Errorf("pulse.Cron = %q, want %q", pulse.Cron, "*/5 * * * *")
	}

	// Invalid jobs are reported, not fatal.
	if config.errors["broken"] == "" {
		t.Error("expected error for job 'broken'")
	}
	if config.errors["both"] == "" {
		t.Error("expected error for job 'both' (schedule + named fields)")
	}
	if config.errors["nocommand"] == "" {
		t.Error("expected error for job 'nocommand' (missing command)")
	}
}

func TestParseJobsFileInvalidTOML(t *testing.T) {
	if _, err := parseJobsFile([]byte("this is not [valid toml")); err == nil {
		t.Error("expected error for invalid TOML")
	}
}

func TestParseJobsFileEmpty(t *testing.T) {
	config, err := parseJobsFile([]byte(""))
	if err != nil {
		t.Fatalf("parseJobsFile: %v", err)
	}
	if len(config.jobs) != 0 {
		t.Errorf("expected no jobs, got %d", len(config.jobs))
	}
}

func TestNormalizeField(t *testing.T) {
	tests := []struct {
		value any
		want  string
		ok    bool
	}{
		{nil, "", true},
		{"5", "5", true},
		{" */5 ", "*/5", true},
		{int64(5), "5", true},
		{uint64(5), "5", true},
		{float64(5), "5", true},
		{float64(5.5), "", false},
		{true, "", false},
	}
	for _, tc := range tests {
		got, err := normalizeField(tc.value)
		if tc.ok && err != nil {
			t.Errorf("normalizeField(%v): unexpected error %v", tc.value, err)
			continue
		}
		if !tc.ok {
			if err == nil {
				t.Errorf("normalizeField(%v): expected error", tc.value)
			}
			continue
		}
		if got != tc.want {
			t.Errorf("normalizeField(%v) = %q, want %q", tc.value, got, tc.want)
		}
	}
}
