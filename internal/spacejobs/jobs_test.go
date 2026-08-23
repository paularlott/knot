package spacejobs

import (
	"testing"

	"github.com/paularlott/knot/internal/database/model"
)

func TestValidateJobs(t *testing.T) {
	errs := ValidateJobs([]model.SpaceJob{
		{Name: "backup", Command: "./backup.sh", Schedule: "0 2 * * *", Enabled: true},
		{Name: "build", Command: "make nightly", Schedule: "30 9 * * 1-5", Enabled: false},
		{Name: "cleanup", Command: "./clean.sh", Enabled: true},
		{Name: "broken", Command: "./broken.sh", Schedule: "not-a-cron", Enabled: true},
		{Name: "nocommand", Schedule: "30 * * * *", Enabled: true},
		{Name: "", Command: "true", Enabled: true},
	})

	if len(errs) != 3 {
		t.Fatalf("expected 3 invalid jobs, got %d: %v", len(errs), errs)
	}
	if errs["broken"] == "" {
		t.Error("expected error for job 'broken' (invalid cron)")
	}
	if errs["nocommand"] == "" {
		t.Error("expected error for job 'nocommand' (missing command)")
	}
	if errs[""] == "" {
		t.Error("expected error for the unnamed job")
	}
}

func TestValidateJobsEmpty(t *testing.T) {
	if errs := ValidateJobs(nil); len(errs) != 0 {
		t.Errorf("expected no errors for no jobs, got %v", errs)
	}
	if errs := ValidateJobs([]model.SpaceJob{}); len(errs) != 0 {
		t.Errorf("expected no errors for empty jobs, got %v", errs)
	}
}

func TestBuildConfigNormalizesSchedule(t *testing.T) {
	config := buildConfig([]model.SpaceJob{
		{Name: "spaced", Command: "true", Schedule: "  */5   *  * *  * ", Enabled: true},
		{Name: "manual", Command: "true", Enabled: true},
	})

	byName := map[string]*Job{}
	for _, job := range config.jobs {
		byName[job.Name] = job
	}
	if len(byName) != 2 {
		t.Fatalf("expected 2 valid jobs, got %d", len(byName))
	}

	if got := byName["spaced"].Cron; got != "*/5 * * * *" {
		t.Errorf("spaced.Cron = %q, want normalized %q", got, "*/5 * * * *")
	}
	if !byName["manual"].ManualOnly || byName["manual"].Cron != "" {
		t.Errorf("job without a schedule should be manual-only, got cron %q", byName["manual"].Cron)
	}
}
