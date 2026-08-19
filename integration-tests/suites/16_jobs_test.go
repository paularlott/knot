//go:build integration

package suites

import (
	"strings"
	"testing"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/integration-tests/harness"
)

// TestSpaceJobs covers the scheduled jobs feature end to end: a jobs file
// written into the space is picked up by the agent, listed and triggered via
// the space-io API, and the job's effect is visible in the space.
func TestSpaceJobs(t *testing.T) {
	harness.Feature(t, "jobs")

	id := spaceFixture(t, "it-jobs", user1.Id, user1.Client)
	c := user1.Client
	ctx, cancel := testCtx(180)
	defer cancel()

	// Before any file exists the list reports found=false.
	jobs, code, err := c.ListJobs(ctx, id)
	if err != nil {
		t.Fatalf("list jobs: %v (status %d)", err, code)
	}
	if jobs.Found {
		t.Fatal("expected found=false before a jobs file exists")
	}

	// Write a jobs file: one manual-only job, one scheduled job.
	if err := c.WriteSpaceFile(ctx, id, ".knot-jobs.toml", `
[jobs.marker]
command = "echo jobs-it-ran > jobs-it-marker.txt"

[jobs.nightly]
command = "true"
hour = 2
minute = 0

[jobs.broken]
command = "./broken.sh"
minute = "not-a-number"
`); err != nil {
		t.Fatalf("write jobs file: %v", err)
	}

	// The agent reports the jobs file in its state (drives the UI icon),
	// with the job runner disabled — it defaulted off because the file was
	// created after the space started.
	waitFor(t, 60, func() bool {
		ctx, cancel := testCtx(15)
		defer cancel()
		space, _, err := c.GetSpace(ctx, id)
		return err == nil && space != nil && space.HasJobs && !space.JobsEnabled
	})

	// The runner started stopped (no file at space start) — the list shows
	// the jobs but reports the runner as disabled.
	jobs, code, err = c.ListJobs(ctx, id)
	if err != nil {
		t.Fatalf("list jobs: %v (status %d)", err, code)
	}
	if !jobs.Found {
		t.Fatal("expected found=true after writing the jobs file")
	}
	if jobs.Enabled {
		t.Fatal("runner should default to stopped when the file was created after start")
	}
	byName := map[string]apiclient.JobInfo{}
	for _, job := range jobs.Jobs {
		byName[job.Name] = job
	}
	mustEqual(t, "job count", len(jobs.Jobs), 3)

	marker := byName["marker"]
	if !marker.ManualOnly || marker.Schedule != "" {
		t.Errorf("marker should be manual-only, got %+v", marker)
	}
	if marker.NextRun != nil {
		t.Errorf("manual-only job should have no next run, got %v", marker.NextRun)
	}

	// Enable the runner (the add-file-to-running-space flow); scheduled jobs
	// now report a next run.
	resp, code, err := c.SetJobEnabled(ctx, id, &apiclient.JobSetEnabledRequest{Enabled: true})
	if err != nil {
		t.Fatalf("enable job runner: %v (status %d)", err, code)
	}
	if !resp.Success {
		t.Fatalf("enable job runner failed: %s", resp.Error)
	}

	jobs, _, err = c.ListJobs(ctx, id)
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	if !jobs.Enabled {
		t.Fatal("runner should be enabled after the enable call")
	}

	// The space state reports the runner as enabled too (the UI's query path).
	waitFor(t, 60, func() bool {
		ctx, cancel := testCtx(15)
		defer cancel()
		space, _, err := c.GetSpace(ctx, id)
		return err == nil && space != nil && space.JobsEnabled
	})

	nightly := byName["nightly"]
	mustEqual(t, "nightly schedule", nightly.Schedule, "0 2 * * *")

	if byName["broken"].Error == "" {
		t.Error("broken job should report a validation error")
	}

	// Trigger the manual job and wait for a successful run record.
	resp, code, err = c.RunJob(ctx, id, &apiclient.JobRunRequest{Name: "marker"})
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

	// Disable the runner via the API; scheduled firing stops but the jobs
	// stay listed and manual triggering keeps working.
	resp, code, err = c.SetJobEnabled(ctx, id, &apiclient.JobSetEnabledRequest{Enabled: false})
	if err != nil {
		t.Fatalf("disable job runner: %v (status %d)", err, code)
	}
	if !resp.Success {
		t.Fatalf("disable job runner failed: %s", resp.Error)
	}

	waitFor(t, 30, func() bool {
		ctx, cancel := testCtx(15)
		defer cancel()
		jobs, _, err := c.ListJobs(ctx, id)
		if err != nil || !jobs.Found || jobs.Enabled {
			return false
		}
		ctx2, cancel2 := testCtx(15)
		defer cancel2()
		space, _, err := c.GetSpace(ctx2, id)
		return err == nil && space != nil && space.HasJobs && !space.JobsEnabled
	})

	// Manual trigger still works with the runner stopped.
	if _, code, err := c.RunJob(ctx, id, &apiclient.JobRunRequest{Name: "marker"}); err != nil {
		t.Fatalf("manual run with stopped runner: %v (status %d)", err, code)
	}

	// Unknown jobs are rejected.
	if _, code, err := c.RunJob(ctx, id, &apiclient.JobRunRequest{Name: "does-not-exist"}); err == nil {
		t.Error("expected error running unknown job")
	} else {
		_ = code
	}

	// Unknown jobs are rejected.
	if _, code, err := c.RunJob(ctx, id, &apiclient.JobRunRequest{Name: "does-not-exist"}); err == nil {
		t.Error("expected error running unknown job")
	} else {
		_ = code
	}
}

// TestSpaceJobsStoppedSpace proves the endpoints behave when the space is not
// running: there is no agent to answer, so the API returns a conflict.
func TestSpaceJobsStoppedSpace(t *testing.T) {
	harness.Feature(t, "jobs")

	id := spaceFixture(t, "it-jobs-stopped", user1.Id, user1.Client)
	c := user1.Client

	harness.StopSpaceAndWait(t, c, id)

	ctx, cancel := testCtx(60)
	defer cancel()

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
