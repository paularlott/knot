// Package syncproject describes sync projects: one or more sync jobs, each
// keeping a local directory and a bucket folder in step, run together.
//
// A project comes from a TOML file (knot file sync-project) or from the
// sync block of a template's volumes, which the server hands the space's
// agent when it registers.
package syncproject

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

// Modes a job syncs in.
const (
	ModeTwoWay   = "two-way"  // changes either side are made on the other
	ModeDownload = "download" // the bucket folder to the directory
	ModeUpload   = "upload"   // the directory to the bucket folder
)

// TemplateProject is the name of the project a template's sync block makes.
const TemplateProject = "template"

// Project is a set of sync jobs run together.
type Project struct {
	// Name names the project; a file's defaults to the file's name.
	Name string `toml:"name" json:"name" msgpack:"name"`
	// Exclude is left out of every job, as well as each job's own.
	Exclude []string `toml:"exclude" json:"exclude,omitempty" msgpack:"exclude,omitempty"`
	// NoDefaultExcludes has every job sync version control folders (.git)
	// and editor scratch files too, leaving out only what Exclude and the
	// jobs' own patterns say.
	NoDefaultExcludes bool  `toml:"no_default_excludes" json:"no_default_excludes,omitempty" msgpack:"no_default_excludes,omitempty"`
	Sync              []Job `toml:"sync" json:"sync" msgpack:"sync"`
}

// Job keeps one local directory and one bucket folder in step.
type Job struct {
	// Name names the job within its project; it defaults to the local
	// directory's name.
	Name string `toml:"name" yaml:"name" json:"name,omitempty" msgpack:"name,omitempty"`
	// Local is the directory; ~ is the home directory of whoever runs it.
	Local string `toml:"local" yaml:"local" json:"local" msgpack:"local"`
	// Remote is the bucket folder, bucket:folder.
	Remote string `toml:"remote" yaml:"remote" json:"remote" msgpack:"remote"`
	// Snapshot, bucket:folder, is snapshotted to Remote when Remote does not
	// exist yet: a new folder starts as a copy of it.
	Snapshot string `toml:"snapshot" yaml:"snapshot" json:"snapshot,omitempty" msgpack:"snapshot,omitempty"`
	// Mode is two-way (the default), download or upload.
	Mode string `toml:"mode" yaml:"mode" json:"mode,omitempty" msgpack:"mode,omitempty"`
	// Watch keeps the job running, syncing changes as they are made; on
	// unless set false.
	Watch *bool `toml:"watch" yaml:"watch" json:"watch,omitempty" msgpack:"watch,omitempty"`
	// Delete passes deletions on; on unless set false.
	Delete *bool `toml:"delete" yaml:"delete" json:"delete,omitempty" msgpack:"delete,omitempty"`
	// Exclude leaves paths out, as knot file sync --exclude.
	Exclude []string `toml:"exclude" yaml:"exclude" json:"exclude,omitempty" msgpack:"exclude,omitempty"`
	// ExcludeFrom names a file of patterns, such as .gitignore, relative
	// to Local, read when the job starts.
	ExcludeFrom string `toml:"exclude_from" yaml:"exclude_from" json:"exclude_from,omitempty" msgpack:"exclude_from,omitempty"`
	// NoDefaultExcludes syncs version control folders (.git) and editor
	// scratch files too.
	NoDefaultExcludes bool `toml:"no_default_excludes" yaml:"no_default_excludes" json:"no_default_excludes,omitempty" msgpack:"no_default_excludes,omitempty"`
	// AllowMassDelete lets a pass delete most of a side's files.
	AllowMassDelete bool `toml:"allow_mass_delete" yaml:"allow_mass_delete" json:"allow_mass_delete,omitempty" msgpack:"allow_mass_delete,omitempty"`
}

// Watching reports whether the job keeps running.
func (j Job) Watching() bool { return j.Watch == nil || *j.Watch }

// Deleting reports whether the job passes deletions on.
func (j Job) Deleting() bool { return j.Delete == nil || *j.Delete }

// SyncMode is the job's mode, two-way when unset.
func (j Job) SyncMode() string {
	if j.Mode == "" {
		return ModeTwoWay
	}
	return j.Mode
}

// Parse reads a project from TOML. name is the project's name when the file
// gives none, such as the file's own name.
func Parse(data []byte, name string) (*Project, error) {
	var p Project
	md, err := toml.Decode(string(data), &p)
	if err != nil {
		return nil, err
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, len(undecoded))
		for i, k := range undecoded {
			keys[i] = k.String()
		}
		return nil, fmt.Errorf("unknown settings: %s", strings.Join(keys, ", "))
	}
	if p.Name == "" {
		p.Name = name
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return &p, nil
}

// NameFromPath is the project name a file gives when it names none: its
// name without the extension.
func NameFromPath(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// Encode writes a project as TOML.
func (p *Project) Encode() ([]byte, error) {
	var b bytes.Buffer
	if err := toml.NewEncoder(&b).Encode(p); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// Validate checks a project, and names its unnamed jobs.
func (p *Project) Validate() error {
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("the project needs a name")
	}
	if strings.ContainsAny(p.Name, `/\`) {
		return fmt.Errorf("project name %q: it cannot hold / or \\", p.Name)
	}
	if len(p.Sync) == 0 {
		return fmt.Errorf("project %s has no sync jobs: add a [[sync]] table", p.Name)
	}
	names := map[string]bool{}
	locals := map[string]string{}
	for i := range p.Sync {
		j := &p.Sync[i]
		if err := j.validate(); err != nil {
			if j.Name != "" {
				return fmt.Errorf("sync job %s: %w", j.Name, err)
			}
			return fmt.Errorf("sync job %d: %w", i+1, err)
		}
		if names[j.Name] {
			return fmt.Errorf("two sync jobs are named %s: give them a name each", j.Name)
		}
		names[j.Name] = true
		local := filepath.Clean(j.Local)
		for other, name := range locals {
			if within(local, other) || within(other, local) {
				return fmt.Errorf("sync jobs %s and %s sync the same directory, or one inside the other", name, j.Name)
			}
		}
		locals[local] = j.Name
	}
	return nil
}

func within(dir, path string) bool {
	return path == dir || strings.HasPrefix(path, strings.TrimSuffix(dir, string(filepath.Separator))+string(filepath.Separator))
}

func (j *Job) validate() error {
	if strings.TrimSpace(j.Local) == "" {
		return fmt.Errorf("local, the directory to sync, is not set")
	}
	if strings.TrimSpace(j.Remote) == "" {
		return fmt.Errorf("remote, the bucket folder to sync, is not set")
	}
	if err := checkRemote("remote", j.Remote); err != nil {
		return err
	}
	if j.Snapshot != "" {
		if err := checkRemote("snapshot", j.Snapshot); err != nil {
			return err
		}
	}
	switch j.SyncMode() {
	case ModeTwoWay, ModeDownload, ModeUpload:
	default:
		return fmt.Errorf("mode %q: it is two-way, download or upload", j.Mode)
	}
	if j.Name == "" {
		j.Name = filepath.Base(filepath.Clean(j.Local))
	}
	return nil
}

// checkRemote checks a bucket:folder.
func checkRemote(field, v string) error {
	bucket, _, ok := strings.Cut(v, ":")
	if !ok || strings.TrimSpace(bucket) == "" {
		return fmt.Errorf("%s %q: give a bucket folder, written bucket:folder", field, v)
	}
	if strings.ContainsAny(v, "*?[") {
		return fmt.Errorf("%s %q: give a folder, not a pattern", field, v)
	}
	return nil
}

// templateSync is the sync block of a template's volumes.
type templateSync struct {
	Sync []Job `yaml:"sync"`
}

// FromVolumes reads the sync block of a template's volumes, its variables
// already resolved: nil when there is none.
func FromVolumes(volumes string) (*Project, error) {
	var ts templateSync
	if err := yaml.Unmarshal([]byte(volumes), &ts); err != nil {
		return nil, err
	}
	if len(ts.Sync) == 0 {
		return nil, nil
	}
	p := &Project{Name: TemplateProject, Sync: ts.Sync}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return p, nil
}

// GitignorePatterns turns the lines of a .gitignore-style file into
// exclude patterns. What exclude patterns cannot say is left out and
// reported: negations (!), which would sync less than the file asks.
// Anchored patterns (/build) match at any depth, which may leave out more.
func GitignorePatterns(data []byte) (patterns, skipped []string) {
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimRight(line, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "!") {
			skipped = append(skipped, trimmed)
			continue
		}
		p := strings.TrimPrefix(trimmed, "/")
		for strings.HasPrefix(p, "**/") {
			p = strings.TrimPrefix(p, "**/")
		}
		p = strings.TrimSuffix(p, "/**")
		if p == "" || p == "**" {
			continue
		}
		patterns = append(patterns, p)
	}
	return patterns, skipped
}

// JobStatus is how a running job is doing.
type JobStatus struct {
	Name   string `json:"name" msgpack:"name"`
	Local  string `json:"local" msgpack:"local"`
	Remote string `json:"remote" msgpack:"remote"`
	Mode   string `json:"mode" msgpack:"mode"`
	// State is starting, snapshotting, syncing, watching, done, failed or
	// stopped.
	State      string `json:"state" msgpack:"state"`
	Passes     int    `json:"passes" msgpack:"passes"`
	LastPass   string `json:"last_pass,omitempty" msgpack:"last_pass,omitempty"` // RFC 3339
	Error      string `json:"error,omitempty" msgpack:"error,omitempty"`
	Uploaded   int    `json:"uploaded" msgpack:"uploaded"`
	Downloaded int    `json:"downloaded" msgpack:"downloaded"`
	Deleted    int    `json:"deleted" msgpack:"deleted"`
	Conflicts  int    `json:"conflicts" msgpack:"conflicts"`
}

// ProjectStatus is how a running project is doing.
type ProjectStatus struct {
	Name string `json:"name" msgpack:"name"`
	// Template says the project is the template's, which the space keeps
	// running.
	Template bool        `json:"template,omitempty" msgpack:"template,omitempty"`
	Jobs     []JobStatus `json:"jobs" msgpack:"jobs"`
}
