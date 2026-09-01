package spacejobs

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/paularlott/knot/internal/database/model"
)

// newTestScheduler builds a scheduler over a temp home with a controllable
// clock, mirroring how Start() configures the default scheduler. Jobs arrive
// via update(), as they do from the server.
func newTestScheduler(t *testing.T, jobs []model.SpaceJob, runnerEnabled bool) *scheduler {
	t.Helper()

	s := &scheduler{
		home:          t.TempDir(),
		running:       map[string]*runningJob{},
		history:       map[string][]RunRecord{},
		now:           time.Now,
		config:        &jobsConfig{errors: map[string]string{}},
		runnerEnabled: runnerEnabled,
	}
	s.update(jobs, runnerEnabled)
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
	s := newTestScheduler(t, []model.SpaceJob{{Name: "touch", Command: "echo hello > " + marker, Enabled: true}}, true)

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
	s := newTestScheduler(t, []model.SpaceJob{{Name: "manual", Command: "true", Enabled: true}}, true)
	snap, err := s.snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	job := snap.Jobs[0]
	if !job.ManualOnly || job.NextRun != nil {
		t.Errorf("manual job should have no next run: %+v", job)
	}
}

func TestSchedulerScheduledFiring(t *testing.T) {
	// A job due "every minute" fired by tick() at a controlled time.
	s := newTestScheduler(t, []model.SpaceJob{{Name: "always", Command: "true", Schedule: "* * * * *", Enabled: true}}, true)

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
	s := newTestScheduler(t, []model.SpaceJob{{Name: "off", Command: "true", Schedule: "* * * * *", Enabled: false}}, true)

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
	s := newTestScheduler(t, []model.SpaceJob{{Name: "job", Command: "true", Schedule: "* * * * *", Enabled: true}}, true)
	snap, _ := s.snapshot()
	if !snap.Enabled {
		t.Fatal("runner should start enabled when pushed enabled")
	}

	// Disabling stops scheduled firing but keeps the job listed.
	s.update([]model.SpaceJob{{Name: "job", Command: "true", Schedule: "* * * * *", Enabled: true}}, false)
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
	s.update([]model.SpaceJob{{Name: "job", Command: "true", Schedule: "* * * * *", Enabled: true}}, true)
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

func TestSchedulerUpdateReplacesDefinitions(t *testing.T) {
	// Jobs pushed after start are live immediately; a later push replaces the
	// set entirely (the space record is the source of truth).
	s := newTestScheduler(t, nil, false)
	snap, _ := s.snapshot()
	if len(snap.Jobs) != 0 {
		t.Fatalf("expected no jobs at start: %+v", snap)
	}

	s.update([]model.SpaceJob{{Name: "late", Command: "true", Schedule: "* * * * *", Enabled: true}}, true)
	snap, err := s.snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if !snap.Enabled || len(snap.Jobs) != 1 || snap.Jobs[0].Name != "late" {
		t.Fatalf("pushed jobs should be live: %+v", snap)
	}

	// A push removing the job stops it firing.
	now := time.Date(2026, 8, 19, 11, 0, 0, 0, time.Local)
	s.now = func() time.Time { return now }
	s.update(nil, true)
	s.tick()
	if hist := s.history["late"]; len(hist) != 0 {
		t.Errorf("removed job must not fire, history: %+v", hist)
	}
	snap, _ = s.snapshot()
	if len(snap.Jobs) != 0 {
		t.Errorf("expected no jobs after empty push: %+v", snap)
	}
	if err := s.runJob("late"); err == nil {
		t.Error("running with no jobs should error")
	}
}

func TestSchedulerUpdatePrunesOrphanedHistory(t *testing.T) {
	// History only exists for current jobs: renaming or deleting a job
	// drops its old-name history instead of leaving it as invisible
	// baggage until the agent restarts.
	s := newTestScheduler(t, []model.SpaceJob{
		{Name: "old", Command: "true", Enabled: true},
		{Name: "kept", Command: "true", Enabled: true},
	}, false)

	record := func(name string) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.recordLocked(name, RunRecord{Status: StatusSuccess, Trigger: TriggerManual})
	}
	record("old")
	record("kept")
	record("gone") // never a current job (e.g. renamed away earlier)

	// Rename "old" → "new" and delete nothing else: old-name history goes,
	// the new name starts fresh, other jobs keep theirs.
	s.update([]model.SpaceJob{
		{Name: "new", Command: "true", Enabled: true},
		{Name: "kept", Command: "true", Enabled: true},
	}, false)
	if _, ok := s.history["old"]; ok {
		t.Error("renamed job's old-name history should be pruned")
	}
	if _, ok := s.history["gone"]; ok {
		t.Error("history for a name with no job should be pruned")
	}
	if hist := s.history["kept"]; len(hist) != 1 {
		t.Errorf("current job's history must survive the update: %+v", hist)
	}
	if hist := s.history["new"]; len(hist) != 0 {
		t.Errorf("renamed job should start with fresh history: %+v", hist)
	}

	// An invalid definition still counts as current: its name keeps its
	// history (the entry is listed with an error, not removed).
	s.update([]model.SpaceJob{
		{Name: "new", Command: "true", Schedule: "not-a-cron", Enabled: true},
		{Name: "kept", Command: "true", Enabled: true},
	}, false)
	record("new")
	s.update([]model.SpaceJob{
		{Name: "new", Command: "true", Enabled: true},
		{Name: "kept", Command: "true", Enabled: true},
	}, false)
	if hist := s.history["new"]; len(hist) != 1 {
		t.Errorf("history of a job that was invalid in between should survive: %+v", hist)
	}

	// Deleting every job clears history entirely.
	s.update(nil, false)
	if len(s.history) != 0 {
		t.Errorf("empty definition set should clear all history: %+v", s.history)
	}
}

func TestSchedulerUnknownJob(t *testing.T) {
	s := newTestScheduler(t, []model.SpaceJob{{Name: "real", Command: "true", Enabled: true}}, true)
	if err := s.runJob("nope"); err == nil {
		t.Error("expected error running unknown job")
	}
}

func TestSchedulerInvalidPushSkipsBadJobs(t *testing.T) {
	// The server validates before pushing, but a bad entry that slips through
	// is skipped and reported rather than blocking the valid ones.
	s := newTestScheduler(t, []model.SpaceJob{
		{Name: "good", Command: "true", Schedule: "* * * * *", Enabled: true},
		{Name: "broken", Command: "true", Schedule: "not-a-cron", Enabled: true},
		{Name: "empty", Enabled: true},
	}, true)

	snap, err := s.snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if len(snap.Jobs) != 3 {
		t.Fatalf("expected all three jobs listed (invalid ones with errors): %+v", snap.Jobs)
	}

	now := time.Date(2026, 8, 19, 11, 0, 0, 0, time.Local)
	s.now = func() time.Time { return now }
	s.tick()
	if hist := s.history["good"]; len(hist) != 1 {
		t.Errorf("valid job should fire, history: %+v", hist)
	}
	if hist := s.history["broken"]; len(hist) != 0 {
		t.Errorf("invalid job must not fire, history: %+v", hist)
	}

	// The invalid entries report their validation error in the snapshot.
	byName := map[string]JobStatus{}
	for _, j := range snap.Jobs {
		byName[j.Name] = j
	}
	if byName["broken"].Error == "" {
		t.Error("broken job should carry a validation error")
	}
	if byName["empty"].Error == "" {
		t.Error("job without a command should carry a validation error")
	}
}

func TestSchedulerDuplicateNamesRejected(t *testing.T) {
	errs := ValidateJobs([]model.SpaceJob{
		{Name: "dup", Command: "true", Enabled: true},
		{Name: "dup", Command: "true", Enabled: true},
	})
	if errs["dup"] == "" {
		t.Error("duplicate job name should be reported")
	}
}

func TestSchedulerLoopTicksAlignedToBoundary(t *testing.T) {
	// The tick loop must fire aligned to the interval boundary, not to the
	// scheduler's start offset: a ticker anchored at start time would delay
	// every scheduled job by up to a full interval.
	s := newTestScheduler(t, nil, false)
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
