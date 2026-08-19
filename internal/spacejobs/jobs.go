package spacejobs

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// JobsFileName is the jobs definition file, looked up in the user's home
// directory. Authored by the user (or installed by a template); the agent only
// ever reads it — enable/disable state lives in a separate sidecar file.
const JobsFileName = ".knot-jobs.toml"

// Job is one validated job definition.
type Job struct {
	Name           string
	Command        string
	Cron           string // normalized 5-field expression, empty for manual-only jobs
	ManualOnly     bool
	sched          *cronSchedule
	EnabledDefault bool // authored enabled value, default true
}

// rawJob mirrors one [jobs.<name>] table. Schedule fields accept ints or
// strings so both `minute = 5` and `minute = "*/5"` work.
type rawJob struct {
	Command  string `toml:"command"`
	Schedule string `toml:"schedule"`
	Minute   any    `toml:"minute"`
	Hour     any    `toml:"hour"`
	Day      any    `toml:"day"`
	Month    any    `toml:"month"`
	Weekday  any    `toml:"weekday"`
	Enabled  *bool  `toml:"enabled"`
}

type rawJobsFile struct {
	Jobs map[string]rawJob `toml:"jobs"`
}

// jobsConfig is the result of parsing a jobs file: valid jobs sorted by name
// plus per-job validation errors for entries that were skipped.
type jobsConfig struct {
	jobs   []*Job
	errors map[string]string
}

// parseJobsFile parses and validates the file body. A TOML syntax error
// returns an error (the caller keeps the previous good config); individual
// invalid jobs are skipped and reported in errors so one bad entry never
// stops the others from running.
func parseJobsFile(data []byte) (*jobsConfig, error) {
	var raw rawJobsFile
	if _, err := toml.Decode(string(data), &raw); err != nil {
		return nil, fmt.Errorf("invalid TOML: %w", err)
	}

	config := &jobsConfig{errors: map[string]string{}}
	for name, entry := range raw.Jobs {
		job, err := validateJob(name, entry)
		if err != nil {
			config.errors[name] = err.Error()
			continue
		}
		config.jobs = append(config.jobs, job)
	}
	sort.Slice(config.jobs, func(i, j int) bool { return config.jobs[i].Name < config.jobs[j].Name })

	return config, nil
}

func validateJob(name string, entry rawJob) (*Job, error) {
	if strings.TrimSpace(entry.Command) == "" {
		return nil, fmt.Errorf("command is required")
	}

	job := &Job{
		Name:           name,
		Command:        entry.Command,
		EnabledDefault: entry.Enabled == nil || *entry.Enabled,
	}

	// The schedule is either a raw 5-field expression or named cron fields —
	// never both. No schedule at all means the job is manual trigger only.
	named := map[string]any{"minute": entry.Minute, "hour": entry.Hour, "day": entry.Day, "month": entry.Month, "weekday": entry.Weekday}
	fields := []string{"", "", "", "", ""} // minute hour day month weekday
	used := 0
	for i, key := range []string{"minute", "hour", "day", "month", "weekday"} {
		value, err := normalizeField(named[key])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		if value != "" {
			fields[i] = value
			used++
		}
	}

	switch {
	case entry.Schedule != "" && used > 0:
		return nil, fmt.Errorf("schedule and named fields (minute/hour/day/month/weekday) are mutually exclusive")
	case entry.Schedule != "":
		job.Cron = strings.Join(strings.Fields(entry.Schedule), " ")
	default:
		if used == 0 {
			job.ManualOnly = true
			return job, nil
		}
		for i, value := range fields {
			if value == "" {
				fields[i] = "*"
			}
		}
		job.Cron = strings.Join(fields, " ")
	}

	sched, err := parseCron(job.Cron)
	if err != nil {
		return nil, fmt.Errorf("invalid schedule %q: %w", job.Cron, err)
	}
	job.sched = sched

	return job, nil
}

// normalizeField converts a schedule field value to its string form. Values
// may be given as ints (`minute = 5`) or strings (`minute = "*/5"`).
func normalizeField(value any) (string, error) {
	switch v := value.(type) {
	case nil:
		return "", nil
	case string:
		return strings.TrimSpace(v), nil
	case int64:
		return strconv.FormatInt(v, 10), nil
	case uint64:
		return strconv.FormatUint(v, 10), nil
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10), nil
		}
		return "", fmt.Errorf("must be an integer or string, got %v", v)
	default:
		return "", fmt.Errorf("must be an integer or string, got %T", value)
	}
}
