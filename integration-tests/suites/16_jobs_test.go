//go:build integration

package suites

import (
	"strings"
	"testing"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/integration-tests/harness"
	"github.com/paularlott/knot/internal/database/model"
)

// TestSpaceJobs covers the scheduled jobs feature end to end: definitions are
// stored on the space and pushed to the agent, listed and triggered via the
// API, and the job's effect is visible in the space.
func TestSpaceJobs(t *testing.T) {
	harness.Feature(t, "jobs")

	id := spaceFixture(t, "it-jobs", user1.Id, user1.Client)
	c := user1.Client
	ctx, cancel := testCtx(180)
	defer cancel()

	// Before any jobs are defined the space reports none and the runner
	// state is carried by the space record.
	defs, code, err := c.GetSpaceJobs(ctx, id)
	if err != nil {
		t.Fatalf("get jobs: %v (status %d)", err, code)
	}
	if len(defs.Jobs) != 0 {
		t.Fatalf("expected no jobs at start, got %+v", defs.Jobs)
	}
	space, _, err := c.GetSpace(ctx, id)
	if err != nil {
		t.Fatalf("get space: %v", err)
	}
	if space.HasJobs || space.JobsEnabled {
		t.Fatalf("space should report no jobs: has=%v enabled=%v", space.HasJobs, space.JobsEnabled)
	}

	// Invalid definitions are rejected wholesale with per-job errors.
	if _, code, err := c.UpdateSpaceJobs(ctx, id, &apiclient.SpaceJobsRequest{
		Jobs: []model.SpaceJob{
			{Name: "good", Command: "true", Enabled: true},
			{Name: "broken", Command: "true", Schedule: "not-a-cron", Enabled: true},
		},
		Enabled: true,
	}); err == nil {
		t.Fatal("expected invalid definitions to be rejected")
	} else {
		mustEqual(t, "invalid update status", code, 400)
		if !strings.Contains(err.Error(), "broken") {
			t.Errorf("error should name the bad job: %v", err)
		}
	}

	// Nothing was saved by the rejected update.
	defs, _, err = c.GetSpaceJobs(ctx, id)
	if err != nil {
		t.Fatalf("get jobs: %v", err)
	}
	if len(defs.Jobs) != 0 {
		t.Fatalf("rejected update must not save, got %+v", defs.Jobs)
	}

	// Define jobs: one manual-only, one scheduled.
	defs, code, err = c.UpdateSpaceJobs(ctx, id, &apiclient.SpaceJobsRequest{
		Jobs: []model.SpaceJob{
			{Name: "marker", Command: "echo jobs-it-ran > jobs-it-marker.txt", Enabled: true},
			{Name: "nightly", Command: "true", Schedule: "0 2 * * *", Enabled: true},
		},
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("update jobs: %v (status %d)", err, code)
	}
	if len(defs.Jobs) != 2 || !defs.Enabled {
		t.Fatalf("unexpected definitions: %+v", defs)
	}

	// The space now reports jobs (drives the UI icon).
	waitFor(t, 60, func() bool {
		ctx, cancel := testCtx(15)
		defer cancel()
		space, _, err := c.GetSpace(ctx, id)
		return err == nil && space != nil && space.HasJobs && space.JobsEnabled
	})

	// The agent lists the pushed jobs with the runner enabled.
	jobs, code, err := c.ListJobs(ctx, id)
	if err != nil {
		t.Fatalf("list jobs: %v (status %d)", err, code)
	}
	if !jobs.Enabled {
		t.Fatal("runner should be enabled")
	}
	byName := map[string]apiclient.JobInfo{}
	for _, job := range jobs.Jobs {
		byName[job.Name] = job
	}
	mustEqual(t, "job count", len(jobs.Jobs), 2)

	marker := byName["marker"]
	if !marker.ManualOnly || marker.Schedule != "" {
		t.Errorf("marker should be manual-only, got %+v", marker)
	}
	if marker.NextRun != nil {
		t.Errorf("manual-only job should have no next run, got %v", marker.NextRun)
	}
	mustEqual(t, "nightly schedule", byName["nightly"].Schedule, "0 2 * * *")
	if byName["nightly"].NextRun == nil {
		t.Error("scheduled job should report a next run")
	}

	// Trigger the manual job and wait for a successful run record.
	resp, code, err := c.RunJob(ctx, id, &apiclient.JobRunRequest{Name: "marker"})
	if err != nil {
		t.Fatalf("run job: %v (status %d)", err, code)
	}
	if !resp.Success {
		t.Fatalf("run job failed: %s", resp.Error)
	}

	var last *apiclient.JobRunRecord
	waitFor(t, 60, func() bool {
		ctx, cancel := testCtx(15)
		defer cancel()
		jobs, _, err := c.ListJobs(ctx, id)
		if err != nil {
			return false
		}
		for _, job := range jobs.Jobs {
			if job.Name == "marker" {
				last = job.LastRun
			}
		}
		return last != nil && last.Status == "success"
	})
	if last == nil {
		t.Fatal("marker job never reported a run")
	}
	if last.Trigger != "manual" {
		t.Errorf("trigger = %q, want manual", last.Trigger)
	}

	// The job actually ran in the space: its output file exists.
	content, err := c.ReadSpaceFile(ctx, id, "jobs-it-marker.txt")
	if err != nil {
		t.Fatalf("read marker file: %v", err)
	}
	if strings.TrimSpace(content) != "jobs-it-ran" {
		t.Errorf("marker file content = %q, want %q", content, "jobs-it-ran")
	}

	// Disable the runner via the definitions; scheduled firing stops but the
	// jobs stay listed and manual triggering keeps working.
	if _, code, err := c.UpdateSpaceJobs(ctx, id, &apiclient.SpaceJobsRequest{
		Jobs: []model.SpaceJob{
			{Name: "marker", Command: "echo jobs-it-ran > jobs-it-marker.txt", Enabled: true},
			{Name: "nightly", Command: "true", Schedule: "0 2 * * *", Enabled: true},
		},
		Enabled: false,
	}); err != nil {
		t.Fatalf("disable job runner: %v (status %d)", err, code)
	}

	waitFor(t, 30, func() bool {
		ctx, cancel := testCtx(15)
		defer cancel()
		jobs, _, err := c.ListJobs(ctx, id)
		if err != nil || jobs.Enabled || len(jobs.Jobs) != 2 {
			return false
		}
		for _, job := range jobs.Jobs {
			if job.Name == "nightly" && job.NextRun != nil {
				return false
			}
		}
		return true
	})
	space, _, err = c.GetSpace(ctx, id)
	if err != nil {
		t.Fatalf("get space: %v", err)
	}
	if !space.HasJobs || space.JobsEnabled {
		t.Fatalf("space should report jobs with runner stopped: has=%v enabled=%v", space.HasJobs, space.JobsEnabled)
	}

	// Manual trigger still works with the runner stopped.
	if _, code, err := c.RunJob(ctx, id, &apiclient.JobRunRequest{Name: "marker"}); err != nil {
		t.Fatalf("manual run with stopped runner: %v (status %d)", err, code)
	}

	// Unknown jobs are rejected.
	if _, _, err := c.RunJob(ctx, id, &apiclient.JobRunRequest{Name: "does-not-exist"}); err == nil {
		t.Error("expected error running unknown job")
	}
}

// TestSpaceJobsStoppedSpace proves the definition endpoints work while the
// space is stopped (definitions live on the space record), while the live
// endpoints need a running agent and return a conflict.
func TestSpaceJobsStoppedSpace(t *testing.T) {
	harness.Feature(t, "jobs")

	id := spaceFixture(t, "it-jobs-stopped", user1.Id, user1.Client)
	c := user1.Client

	// Define a job and stop the space; the definitions survive.
	ctx, cancel := testCtx(60)
	defer cancel()
	if _, code, err := c.UpdateSpaceJobs(ctx, id, &apiclient.SpaceJobsRequest{
		Jobs: []model.SpaceJob{
			{Name: "marker", Command: "true", Enabled: true},
		},
		Enabled: true,
	}); err != nil {
		t.Fatalf("update jobs: %v (status %d)", err, code)
	}

	harness.StopSpaceAndWait(t, c, id)

	defs, code, err := c.GetSpaceJobs(ctx, id)
	if err != nil {
		t.Fatalf("get jobs of stopped space: %v (status %d)", err, code)
	}
	if len(defs.Jobs) != 1 || !defs.Enabled {
		t.Fatalf("definitions should be readable while stopped: %+v", defs)
	}

	// Definitions can be updated while stopped; the agent picks them up on
	// its next registration.
	if _, code, err := c.UpdateSpaceJobs(ctx, id, &apiclient.SpaceJobsRequest{
		Jobs: []model.SpaceJob{
			{Name: "marker", Command: "true", Enabled: true},
			{Name: "added", Command: "true", Schedule: "*/5 * * * *", Enabled: true},
		},
		Enabled: true,
	}); err != nil {
		t.Fatalf("update jobs of stopped space: %v (status %d)", err, code)
	}

	if _, code, err := c.ListJobs(ctx, id); err == nil {
		t.Fatal("expected error listing jobs of stopped space")
	} else {
		mustEqual(t, "list jobs status", code, 409)
	}

	if _, code, err := c.RunJob(ctx, id, &apiclient.JobRunRequest{Name: "marker"}); err == nil {
		t.Fatal("expected error running job in stopped space")
	} else {
		mustEqual(t, "run job status", code, 409)
	}
}
