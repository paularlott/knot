package spacejobs

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/util"

	"github.com/paularlott/logger"
)

const historyLimit = 10

// Run statuses and triggers for a job's history entries.
const (
	StatusRunning = "running"
	StatusSuccess = "success"
	StatusFailed  = "failed"
	StatusSkipped = "skipped"

	TriggerSchedule = "schedule"
	TriggerManual   = "manual"
)

// RunRecord is one entry in a job's bounded run history. Kept in memory only —
// history dies with the space, matching the "no catch-up, nothing owed while
// stopped" model.
type RunRecord struct {
	StartedAt  time.Time `json:"started_at" msgpack:"started_at"`
	FinishedAt time.Time `json:"finished_at" msgpack:"finished_at"`
	DurationMs int64     `json:"duration_ms" msgpack:"duration_ms"`
	Status     string    `json:"status" msgpack:"status"`
	Trigger    string    `json:"trigger" msgpack:"trigger"`
	Error      string    `json:"error,omitempty" msgpack:"error,omitempty"`
}

// JobStatus is the wire view of one job.
type JobStatus struct {
	Name       string     `json:"name" msgpack:"name"`
	Command    string     `json:"command" msgpack:"command"`
	Schedule   string     `json:"schedule,omitempty" msgpack:"schedule,omitempty"`
	ManualOnly bool       `json:"manual_only" msgpack:"manual_only"`
	Enabled    bool       `json:"enabled" msgpack:"enabled"`
	Running    bool       `json:"running" msgpack:"running"`
	NextRun    *time.Time `json:"next_run,omitempty" msgpack:"next_run,omitempty"`
	LastRun    *RunRecord `json:"last_run,omitempty" msgpack:"last_run,omitempty"`
	Error      string     `json:"error,omitempty"`
}

// JobsSnapshot is the full view returned by list commands and the API.
// Enabled reports the job runner state pushed from the server: the space
// record is the source of truth and every change is pushed live.
type JobsSnapshot struct {
	Enabled bool        `json:"enabled" msgpack:"enabled"`
	Jobs    []JobStatus `json:"jobs" msgpack:"jobs"`
}

type runningJob struct {
	cancel  context.CancelFunc
	started time.Time
	trigger string
}

type scheduler struct {
	mu sync.Mutex

	home string

	config *jobsConfig
	// runnerEnabled mirrors the space's persisted jobs_enabled flag, pushed
	// from the server with the job definitions.
	runnerEnabled bool
	running       map[string]*runningJob
	history       map[string][]RunRecord
	lastTick      int64 // minute bucket of the last evaluated tick

	now          func() time.Time
	tickInterval time.Duration

	stopCh   chan struct{}
	stopOnce sync.Once
}

var defaultScheduler = &scheduler{
	running: map[string]*runningJob{},
	history: map[string][]RunRecord{},
	now:     time.Now,
}

// jobLogger is where run activity goes. The agent daemon replaces it with the
// agent client's uplink logger so job output reaches the space's log window;
// by default it writes to the standard (stderr) logger.
var jobLogger logger.Logger = log.GetLogger()

// SetLogger installs the logger used for job activity and output. Call before
// Start.
func SetLogger(l logger.Logger) {
	if l != nil {
		jobLogger = l
	}
}

// Start starts the default scheduler in the user's home directory and begins
// firing due jobs every minute. Jobs themselves arrive later: the server
// pushes them on registration and on every change. Safe to call once per
// process; the agent daemon calls it at startup.
func Start() {
	home, err := os.UserHomeDir()
	if err != nil {
		jobLogger.WithError(err).Error("jobs: failed to resolve home directory, scheduler not started")
		return
	}

	defaultScheduler.start(home, time.Minute)
}

// Stop halts the default scheduler and terminates any running jobs.
func Stop() {
	defaultScheduler.stop()
}

// Snapshot returns the current state of all jobs.
func Snapshot() (*JobsSnapshot, error) {
	return defaultScheduler.snapshot()
}

// RunJob starts a job immediately by name. Manual triggering always works —
// neither the job's `enabled = false` nor a stopped runner blocks it, since
// `enabled` only gates automatic firing.
func RunJob(name string) error {
	return defaultScheduler.runJob(name)
}

// Update replaces the scheduler's job definitions and runner state with the
// latest push from the server. Valid jobs are applied immediately; entries
// that fail validation are skipped and reported in the snapshot rather than
// blocking the rest.
func Update(defs []model.SpaceJob, runnerEnabled bool) {
	defaultScheduler.update(defs, runnerEnabled)
}

func (s *scheduler) start(home string, tickInterval time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.home = home
	s.tickInterval = tickInterval
	s.stopCh = make(chan struct{})
	s.stopOnce = sync.Once{}
	s.config = &jobsConfig{errors: map[string]string{}}

	go s.loop()
	jobLogger.Info("jobs: scheduler started", "runner_enabled", s.runnerEnabled)
}

func (s *scheduler) update(defs []model.SpaceJob, runnerEnabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.config = buildConfig(defs)
	s.runnerEnabled = runnerEnabled

	// History only exists for current jobs: drop entries whose names left
	// the config, whether deleted or renamed. Invalid definitions keep
	// their names in errors and still count as current. A run executing
	// under a removed name when the update lands re-adds its final record
	// on completion — bounded by the history limit.
	current := make(map[string]struct{}, len(s.config.jobs)+len(s.config.errors))
	for _, job := range s.config.jobs {
		current[job.Name] = struct{}{}
	}
	for name := range s.config.errors {
		current[name] = struct{}{}
	}
	for name := range s.history {
		if _, ok := current[name]; !ok {
			delete(s.history, name)
		}
	}

	jobLogger.Info("jobs: definitions updated", "jobs", len(s.config.jobs), "invalid", len(s.config.errors), "runner_enabled", runnerEnabled)
}

func (s *scheduler) stop() {
	s.mu.Lock()
	if s.stopCh != nil {
		s.stopOnce.Do(func() { close(s.stopCh) })
	}
	for _, rj := range s.running {
		rj.cancel()
	}
	s.mu.Unlock()
}

// loop drives the scheduler, ticking once per interval aligned to the
// interval boundary (the top of the minute by default) plus a small grace so
// clock adjustments never leave the tick straddling a boundary. Re-anchoring
// every iteration — rather than a plain ticker — keeps the alignment even as
// the clock drifts: without it, a ticker anchored to the agent's start time
// fires at a fixed offset (e.g. always 12s past the minute), delaying every
// scheduled job by that offset.
func (s *scheduler) loop() {
	interval := s.tickInterval
	if interval <= 0 {
		interval = time.Minute
	}

	for {
		now := s.now()
		next := now.Truncate(interval).Add(interval).Add(250 * time.Millisecond)
		select {
		case <-s.stopCh:
			return
		case <-time.After(next.Sub(now)):
			s.tick()
		}
	}
}

// tick evaluates all scheduled jobs against the current minute. Each minute
// is evaluated at most once; minutes that pass while the agent is stopped are
// simply missed — there is no catch-up by design.
func (s *scheduler) tick() {
	s.mu.Lock()
	defer s.mu.Unlock()

	interval := s.tickInterval
	if interval <= 0 {
		interval = time.Minute
	}

	now := s.now()
	bucket := now.Truncate(interval).Unix()
	if bucket == s.lastTick {
		return
	}
	s.lastTick = bucket

	if !s.runnerEnabled {
		return
	}

	for _, job := range s.config.jobs {
		if job.ManualOnly || !job.Enabled {
			continue
		}
		if !job.sched.matches(now) {
			continue
		}

		if _, running := s.running[job.Name]; running {
			s.recordLocked(job.Name, RunRecord{
				StartedAt:  now,
				FinishedAt: now,
				Status:     StatusSkipped,
				Trigger:    TriggerSchedule,
				Error:      "skipped, previous run still active",
			})
			jobLogger.Warn("skipped " + job.Name + ", previous run still active")
			continue
		}

		s.startRunLocked(job, TriggerSchedule)
	}
}

func (s *scheduler) snapshot() (*JobsSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	snap := &JobsSnapshot{Enabled: s.runnerEnabled}

	if s.config != nil {
		now := s.now()
		for _, job := range s.config.jobs {
			status := JobStatus{
				Name:       job.Name,
				Command:    job.Command,
				Schedule:   job.Cron,
				ManualOnly: job.ManualOnly,
				Enabled:    job.Enabled,
				Running:    false,
				Error:      s.config.errors[job.Name],
			}
			if _, ok := s.running[job.Name]; ok {
				status.Running = true
			}
			if s.runnerEnabled && !job.ManualOnly && status.Enabled && job.sched != nil {
				if next, ok := job.sched.next(now); ok {
					status.NextRun = &next
				}
			}
			if hist := s.history[job.Name]; len(hist) > 0 {
				last := hist[len(hist)-1]
				status.LastRun = &last
			}
			snap.Jobs = append(snap.Jobs, status)
		}
	}

	// Jobs skipped for validation errors still appear in the list so `list`
	// and the UI surface the problem instead of hiding the entry.
	if s.config != nil {
		names := make([]string, 0, len(s.config.errors))
		for name := range s.config.errors {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			if s.hasJobLocked(name) {
				continue // already reported above with its error set
			}
			snap.Jobs = append(snap.Jobs, JobStatus{
				Name:    name,
				Enabled: false,
				Error:   s.config.errors[name],
			})
		}
	}

	return snap, nil
}

func (s *scheduler) runJob(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.config == nil || len(s.config.jobs) == 0 {
		return errors.New("no jobs defined")
	}
	if msg, ok := s.config.errors[name]; ok {
		return fmt.Errorf("job %q is invalid: %s", name, msg)
	}

	var job *Job
	for _, j := range s.config.jobs {
		if j.Name == name {
			job = j
			break
		}
	}
	if job == nil {
		return fmt.Errorf("job %q not found", name)
	}
	if _, running := s.running[name]; running {
		return fmt.Errorf("job %q is already running", name)
	}

	s.startRunLocked(job, TriggerManual)

	return nil
}

// startRunLocked spawns the job's command via the user's shell in the home
// directory. Log messages lead with the job name (starting <name>,
// <name>: <output>, completed <name>) so runs are easy to follow in the
// space's logs. The returned record is added to history as running and
// finalised when the process exits.
func (s *scheduler) startRunLocked(job *Job, trigger string) {
	started := s.now()

	shell := util.CheckShells("bash")
	if shell == "" {
		s.recordLocked(job.Name, RunRecord{
			StartedAt:  started,
			FinishedAt: started,
			Status:     StatusFailed,
			Trigger:    trigger,
			Error:      "no valid shell found",
		})
		jobLogger.Error("failed to start " + job.Name + ", no valid shell found")
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, shell, "-c", job.Command)
	cmd.Dir = s.home
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		jobLogger.WithError(err).Error("failed to start " + job.Name)
		return
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		cancel()
		s.recordLocked(job.Name, RunRecord{
			StartedAt:  started,
			FinishedAt: started,
			Status:     StatusFailed,
			Trigger:    trigger,
			Error:      err.Error(),
		})
		jobLogger.WithError(err).Error("failed to start "+job.Name, "command", job.Command)
		return
	}

	s.running[job.Name] = &runningJob{cancel: cancel, started: started, trigger: trigger}
	s.recordLocked(job.Name, RunRecord{
		StartedAt: started,
		Status:    StatusRunning,
		Trigger:   trigger,
	})
	jobLogger.Info("starting "+job.Name, "trigger", trigger)

	go func() {
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			jobLogger.Info(job.Name + ": " + scanner.Text())
		}

		err := cmd.Wait()

		s.mu.Lock()
		finished := s.now()
		if rj, ok := s.running[job.Name]; ok {
			record := RunRecord{
				StartedAt:  rj.started,
				FinishedAt: finished,
				DurationMs: finished.Sub(rj.started).Milliseconds(),
				Status:     StatusFailed,
				Trigger:    rj.trigger,
			}
			if err == nil {
				record.Status = StatusSuccess
			} else {
				record.Error = err.Error()
			}
			s.recordLocked(job.Name, record)
			if rj.cancel != nil {
				rj.cancel()
			}
		}
		delete(s.running, job.Name)
		s.mu.Unlock()

		if err != nil {
			jobLogger.Error("failed "+job.Name, "duration_ms", finished.Sub(started).Milliseconds(), "error", err.Error())
		} else {
			jobLogger.Info("completed "+job.Name, "duration_ms", finished.Sub(started).Milliseconds())
		}
	}()
}

// recordLocked appends a record to the job's bounded run history. Run
// completions are appended as new entries (keyed off the running job, not off
// the history contents) so an intermediate entry — e.g. a skip while the run
// was still active — can never swallow the run's outcome.
func (s *scheduler) recordLocked(name string, record RunRecord) {
	hist := append(s.history[name], record)
	if len(hist) > historyLimit {
		hist = hist[len(hist)-historyLimit:]
	}
	s.history[name] = hist
}

func (s *scheduler) hasJobLocked(name string) bool {
	if s.config == nil {
		return false
	}
	for _, j := range s.config.jobs {
		if j.Name == name {
			return true
		}
	}
	return false
}
