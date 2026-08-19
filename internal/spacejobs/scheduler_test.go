package spacejobs

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// newTestScheduler builds a scheduler over a temp home with a controllable
// clock, mirroring how Start() configures the default scheduler.
func newTestScheduler(t *testing.T, jobsFile string) *scheduler {
	t.Helper()
	home := t.TempDir()
	if jobsFile != "" {
		if err := os.WriteFile(filepath.Join(home, JobsFileName), []byte(jobsFile), 0o644); err != nil {
			t.Fatalf("write jobs file: %v", err)
		}
	}

	s := &scheduler{
		home:     home,
		jobsFile: filepath.Join(home, JobsFileName),
		running:  map[string]*runningJob{},
		history:  map[string][]RunRecord{},
		now:      time.Now,
	}
	s.reloadLocked()
	// Mirror start(): the runner defaults to running when the file exists at
	// startup, stopped when it does not.
	s.runnerEnabled = s.found
	return s
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("condition not met within timeout")
}

func TestSchedulerManualRun(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "ran")
	s := newTestScheduler(t, "[jobs.touch]\ncommand = \"echo hello > "+marker+"\"\n")

	if err := s.runJob("touch"); err != nil {
		t.Fatalf("runJob: %v", err)
	}
	waitFor(t, 5*time.Second, func() bool {
		_, err := os.Stat(marker)
		return err == nil
	})

	// History records a successful manual run.
	waitFor(t, 5*time.Second, func() bool {
		snap, _ := s.snapshot()
		return len(snap.Jobs) == 1 && snap.Jobs[0].LastRun != nil && snap.Jobs[0].LastRun.Status == StatusSuccess
	})
	snap, err := s.snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if len(snap.Jobs) != 1 || snap.Jobs[0].Name != "touch" {
		t.Fatalf("unexpected jobs: %+v", snap.Jobs)
	}
	if snap.Jobs[0].LastRun.Trigger != TriggerManual {
		t.Errorf("last run = %+v, want manual trigger", snap.Jobs[0].LastRun)
	}
}

func TestSchedulerManualOnlyJobHasNoNextRun(t *testing.T) {
	s := newTestScheduler(t, `
[jobs.manual]
command = "true"
`)
	snap, err := s.snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if !snap.Found {
		t.Error("expected file to be found")
	}
	job := snap.Jobs[0]
	if !job.ManualOnly || job.NextRun != nil {
		t.Errorf("manual job should have no next run: %+v", job)
	}
}

func TestSchedulerScheduledFiring(t *testing.T) {
	// A job due "every minute" fired by tick() at a controlled time.
	s := newTestScheduler(t, `
[jobs.always]
command = "true"
minute = "*"
`)

	now := time.Date(2026, 8, 19, 10, 30, 5, 0, time.Local)
	s.now = func() time.Time { return now }
	s.tick()

	snap, err := s.snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snap.Jobs[0].LastRun == nil || snap.Jobs[0].LastRun.Status != StatusRunning && snap.Jobs[0].LastRun.Status != StatusSuccess {
		t.Errorf("job should have been fired by tick: %+v", snap.Jobs[0].LastRun)
	}
	if snap.Jobs[0].LastRun.Trigger != TriggerSchedule {
		t.Errorf("trigger = %q, want schedule", snap.Jobs[0].LastRun.Trigger)
	}

	// The same minute never fires twice.
	s.tick()
	snap, _ = s.snapshot()
	hist := s.history["always"]
	if len(hist) != 1 {
		t.Errorf("expected 1 history entry after duplicate tick, got %d", len(hist))
	}
}

func TestSchedulerDisabledJobDoesNotFire(t *testing.T) {
	s := newTestScheduler(t, `
[jobs.off]
command = "true"
minute = "*"
enabled = false
`)

	now := time.Date(2026, 8, 19, 10, 30, 0, 0, time.Local)
	s.now = func() time.Time { return now }
	s.tick()

	if hist := s.history["off"]; len(hist) != 0 {
		t.Errorf("disabled job should not fire, history: %+v", hist)
	}
	snap, _ := s.snapshot()
	if snap.Jobs[0].Enabled {
		t.Error("job should report disabled")
	}
}

func TestSchedulerRunnerEnableDisable(t *testing.T) {
	// Runner starts enabled because the file exists at start.
	s := newTestScheduler(t, "[jobs.job]\ncommand = \"true\"\nminute = \"*\"\n")
	snap, _ := s.snapshot()
	if !snap.Enabled {
		t.Fatal("runner should default to enabled when the file exists at start")
	}

	// Disabling stops scheduled firing but keeps the job listed.
	if err := s.setEnabled(false); err != nil {
		t.Fatalf("disable: %v", err)
	}
	now := time.Date(2026, 8, 19, 10, 30, 0, 0, time.Local)
	s.now = func() time.Time { return now }
	s.tick()
	if hist := s.history["job"]; len(hist) != 0 {
		t.Errorf("disabled runner must not fire jobs, history: %+v", hist)
	}
	snap, _ = s.snapshot()
	if snap.Enabled || len(snap.Jobs) != 1 || snap.Jobs[0].NextRun != nil {
		t.Errorf("stopped runner should report disabled with no next run: %+v", snap)
	}

	// Manual trigger still works while the runner is stopped.
	if err := s.runJob("job"); err != nil {
		t.Errorf("manual run with stopped runner: %v", err)
	}
	waitFor(t, 5*time.Second, func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		_, running := s.running["job"]
		return !running
	})

	// Re-enabling resumes firing (advance the clock past the minute already
	// consumed by the tick above; the manual run is not a scheduled fire).
	if err := s.setEnabled(true); err != nil {
		t.Fatalf("enable: %v", err)
	}
	now = now.Add(time.Minute)
	s.now = func() time.Time { return now }
	s.tick()
	waitFor(t, 5*time.Second, func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		hist := s.history["job"]
		return len(hist) >= 2 && hist[len(hist)-1].Status == StatusSuccess && hist[len(hist)-1].Trigger == TriggerSchedule
	})
	if hist := s.history["job"]; len(hist) < 2 || hist[len(hist)-1].Trigger != TriggerSchedule {
		t.Errorf("re-enabled runner should fire due jobs, history: %+v", hist)
	}
}

func TestSchedulerRunnerDefaultNotPersisted(t *testing.T) {
	// A jobs file created after start leaves the runner stopped until enabled;
	// restarting the agent (new scheduler over the same home) picks the file
	// up and defaults to running — nothing is persisted anywhere.
	s := newTestScheduler(t, "")
	if err := os.WriteFile(s.jobsFile, []byte("[jobs.late]\ncommand = \"true\"\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	snap, _ := s.snapshot()
	if snap.Enabled {
		t.Fatal("runner should stay stopped when the file appears after start")
	}

	s2 := newTestScheduler(t, "[jobs.late]\ncommand = \"true\"\n")
	s2.home = s.home
	s2.jobsFile = s.jobsFile
	s2.reloadLocked()
	s2.runnerEnabled = s2.found
	snap2, _ := s2.snapshot()
	if !snap2.Enabled {
		t.Fatal("runner should default to enabled after a restart with the file present")
	}
}

func TestSchedulerEnableRequiresFile(t *testing.T) {
	s := newTestScheduler(t, "")
	if err := s.setEnabled(true); err == nil {
		t.Error("enabling without a jobs file should error")
	}
	if err := s.setEnabled(false); err != nil {
		t.Errorf("disabling without a jobs file should succeed: %v", err)
	}
}

func TestSchedulerUnknownJob(t *testing.T) {
	s := newTestScheduler(t, `[jobs.real]
command = "true"
`)
	if err := s.runJob("nope"); err == nil {
		t.Error("expected error running unknown job")
	}
}

func TestSchedulerBrokenFileKeepsPreviousJobs(t *testing.T) {
	s := newTestScheduler(t, `
[jobs.good]
command = "true"
minute = "*"
`)

	// Corrupt the file — the previous good config must stay active.
	if err := os.WriteFile(s.jobsFile, []byte("not [valid"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	s.reloadLocked()

	snap, err := s.snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snap.Error == "" {
		t.Error("expected file-level error to be reported")
	}
	if len(snap.Jobs) != 1 || snap.Jobs[0].Name != "good" {
		t.Errorf("previous good config should be kept, jobs: %+v", snap.Jobs)
	}
}

func TestSchedulerMissingFile(t *testing.T) {
	// No jobs file written, so the scheduler starts with none present.
	s := newTestScheduler(t, "")
	s.reloadLocked()

	snap, err := s.snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snap.Found {
		t.Error("file should be reported as not found")
	}
	if len(snap.Jobs) != 0 {
		t.Errorf("expected no jobs, got %+v", snap.Jobs)
	}
	if err := s.runJob("anything"); err == nil {
		t.Error("running without a jobs file should error")
	}
}

func TestSchedulerFileCreatedAfterStart(t *testing.T) {
	// Space starts with no jobs file; the user creates one afterwards.
	s := newTestScheduler(t, "")

	snap, _ := s.snapshot()
	if snap.Found || len(snap.Jobs) != 0 {
		t.Fatalf("expected no file and no jobs at start: %+v", snap)
	}

	if err := os.WriteFile(s.jobsFile, []byte("[jobs.late]\ncommand = \"true\"\nminute = \"*\"\n"), 0o644); err != nil {
		t.Fatalf("write jobs file: %v", err)
	}

	// The next snapshot (or tick) picks the file up without a restart.
	snap, err := s.snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if !snap.Found || len(snap.Jobs) != 1 || snap.Jobs[0].Name != "late" {
		t.Fatalf("created file should be picked up: %+v", snap)
	}

	// The runner defaulted off (no file at start), so nothing fires yet.
	now := time.Date(2026, 8, 19, 11, 0, 0, 0, time.Local)
	s.now = func() time.Time { return now }
	s.tick()
	if hist := s.history["late"]; len(hist) != 0 {
		t.Errorf("runner should be stopped until enabled, history: %+v", hist)
	}

	// Enabling starts scheduled firing (advance the clock past the minute
	// already consumed by the tick above).
	if err := s.setEnabled(true); err != nil {
		t.Fatalf("enable after file creation: %v", err)
	}
	now = now.Add(time.Minute)
	s.now = func() time.Time { return now }
	s.tick()
	if hist := s.history["late"]; len(hist) != 1 {
		t.Errorf("created job should fire once enabled, history: %+v", hist)
	}

	// Manual run works too (wait for the tick-fired run to finish so the
	// manual run isn't rejected by the overlap guard).
	waitFor(t, 5*time.Second, func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		_, running := s.running["late"]
		return !running
	})
	if err := s.runJob("late"); err != nil {
		t.Errorf("runJob after file creation: %v", err)
	}
}

func TestSchedulerFileDeletedAfterStart(t *testing.T) {
	// Space starts with jobs; the user deletes the file afterwards.
	s := newTestScheduler(t, "[jobs.gone]\ncommand = \"true\"\nminute = \"*\"\n")

	snap, _ := s.snapshot()
	if !snap.Found || len(snap.Jobs) != 1 {
		t.Fatalf("expected the job at start: %+v", snap)
	}

	if err := os.Remove(s.jobsFile); err != nil {
		t.Fatalf("remove jobs file: %v", err)
	}

	// The next snapshot (or tick) drops the jobs, reports not found, and
	// stops the runner.
	snap, err := s.snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snap.Found || len(snap.Jobs) != 0 {
		t.Fatalf("deleted file should remove all jobs: %+v", snap)
	}
	if snap.Enabled {
		t.Fatal("deleting the jobs file should stop the runner")
	}

	// Nothing fires on ticks any more.
	now := time.Date(2026, 8, 19, 11, 0, 0, 0, time.Local)
	s.now = func() time.Time { return now }
	s.tick()
	if hist := s.history["gone"]; len(hist) != 0 {
		t.Errorf("deleted job must not fire, history: %+v", hist)
	}

	// Running reports the missing file; enabling requires the file, disabling
	// always succeeds.
	if err := s.runJob("gone"); err == nil || err.Error() != "no jobs file found" {
		t.Errorf("runJob after deletion = %v, want 'no jobs file found'", err)
	}
	if err := s.setEnabled(true); err == nil || err.Error() != "no jobs file found" {
		t.Errorf("enable after deletion = %v, want 'no jobs file found'", err)
	}
	if err := s.setEnabled(false); err != nil {
		t.Errorf("disable after deletion should succeed: %v", err)
	}

	// Recreating the file brings the jobs back but leaves the runner
	// stopped until explicitly enabled.
	if err := os.WriteFile(s.jobsFile, []byte("[jobs.gone]\ncommand = \"true\"\nminute = \"*\"\n"), 0o644); err != nil {
		t.Fatalf("recreate jobs file: %v", err)
	}
	snap, err = s.snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if !snap.Found || len(snap.Jobs) != 1 || snap.Enabled {
		t.Fatalf("recreated file should list jobs with the runner still stopped: %+v", snap)
	}
	s.tick()
	if hist := s.history["gone"]; len(hist) != 0 {
		t.Errorf("recreated job must not fire while the runner is stopped, history: %+v", hist)
	}
	if err := s.setEnabled(true); err != nil {
		t.Fatalf("enable after recreation: %v", err)
	}
	now = now.Add(time.Minute)
	s.now = func() time.Time { return now }
	s.tick()
	if hist := s.history["gone"]; len(hist) != 1 {
		t.Errorf("job should fire once the runner is re-enabled, history: %+v", hist)
	}
}

func TestSchedulerLoopTicksAlignedToBoundary(t *testing.T) {
	// The tick loop must fire aligned to the interval boundary, not to the
	// scheduler's start offset: a ticker anchored at start time would delay
	// every scheduled job by up to a full interval.
	s := newTestScheduler(t, "")
	s.tickInterval = 500 * time.Millisecond

	observed := make(chan time.Time, 32)

	done := make(chan struct{})
	go func() {
		s.mu.Lock()
		last := s.lastTick
		s.mu.Unlock()
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			s.mu.Lock()
			current := s.lastTick
			s.mu.Unlock()
			if current != last {
				last = current
				select {
				case observed <- time.Now():
				default:
				}
			}
			time.Sleep(10 * time.Millisecond)
		}
		close(done)
	}()

	go s.loop()
	<-done

	close(observed)
	count := 0
	for ts := range observed {
		count++
		// An aligned tick lands just after the boundary (250ms grace plus
		// scheduling slack). A ticker anchored to start time would fire at a
		// fixed offset strictly inside [0, interval). Interval must exceed
		// the grace or every tick spills into the next bucket.
		offset := ts.Sub(ts.Truncate(500 * time.Millisecond))
		if offset < 240*time.Millisecond || offset > 450*time.Millisecond {
			t.Errorf("tick at %v landed %v into its interval, want ~250ms (aligned)", ts, offset)
		}
	}
	if count < 2 {
		t.Errorf("expected several aligned ticks, saw %d", count)
	}
}
