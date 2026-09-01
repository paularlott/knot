package spacejobs

import (
	"fmt"
	"sort"
	"strings"

	"github.com/paularlott/knot/internal/database/model"
)

// Job is one validated job definition held in memory by the scheduler. The
// space record (model.SpaceJob) is the persisted source of truth; the agent
// receives it on registration and on every change, and never writes it back.
type Job struct {
	Name       string
	Command    string
	Cron       string // normalized 5-field expression, empty for manual-only jobs
	ManualOnly bool
	sched      *cronSchedule
	Enabled    bool
}

// jobsConfig is the result of validating a set of job definitions: valid jobs
// sorted by name plus per-job validation errors for entries that were skipped.
type jobsConfig struct {
	jobs   []*Job
	errors map[string]string
}

// ValidateJobs validates a set of job definitions and returns an error for
// each invalid entry keyed by job name. An empty map means every job is valid.
// The server API uses this to reject bad definitions on save; the scheduler
// applies the same validation defensively when a push arrives.
func ValidateJobs(defs []model.SpaceJob) map[string]string {
	config := buildConfig(defs)
	return config.errors
}

// buildConfig validates the definitions. Invalid entries are skipped and
// reported in errors so one bad job never stops the others from running.
func buildConfig(defs []model.SpaceJob) *jobsConfig {
	config := &jobsConfig{errors: map[string]string{}}
	seen := map[string]bool{}
	for _, def := range defs {
		job, err := validateJob(def, seen)
		if err != nil {
			config.errors[def.Name] = err.Error()
			continue
		}
		config.jobs = append(config.jobs, job)
	}
	sort.Slice(config.jobs, func(i, j int) bool { return config.jobs[i].Name < config.jobs[j].Name })

	return config
}

func validateJob(def model.SpaceJob, seen map[string]bool) (*Job, error) {
	name := strings.TrimSpace(def.Name)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if seen[name] {
		return nil, fmt.Errorf("duplicate job name")
	}
	seen[name] = true
	if strings.TrimSpace(def.Command) == "" {
		return nil, fmt.Errorf("command is required")
	}

	job := &Job{
		Name:    name,
		Command: def.Command,
		Enabled: def.Enabled,
	}

	// An empty schedule means the job is manual trigger only.
	job.Cron = strings.Join(strings.Fields(def.Schedule), " ")
	if job.Cron == "" {
		job.ManualOnly = true
		return job, nil
	}

	sched, err := parseCron(job.Cron)
	if err != nil {
		return nil, fmt.Errorf("invalid schedule %q: %w", job.Cron, err)
	}
	job.sched = sched

	return job, nil
}
