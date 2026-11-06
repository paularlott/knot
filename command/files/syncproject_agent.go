package command_files

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/syncproject"
	"github.com/paularlott/logger"
)

// ProjectManager runs sync projects in a space's agent: the template's sync
// block, and the projects started with knot file sync-project start, which
// are kept on disk so that an agent restart starts them again.
type ProjectManager struct {
	ctx    context.Context
	dir    string // where started projects are kept
	client clientFunc
	log    logger.Logger
	follow eventFollower // nil for the server's

	mu       sync.Mutex
	runs     map[string]*projectRun
	template []byte // the template's project as running, encoded
}

// NewProjectManager makes a manager keeping started projects in dir, and
// syncing with the client client gives.
func NewProjectManager(ctx context.Context, dir string, client func() (*apiclient.ApiClient, error), l logger.Logger) *ProjectManager {
	return &ProjectManager{ctx: ctx, dir: dir, client: client, log: l, runs: map[string]*projectRun{}}
}

// Resume starts the projects started before the agent last stopped.
func (m *ProjectManager) Resume() {
	files, _ := filepath.Glob(filepath.Join(m.dir, "*.toml"))
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			m.log.WithError(err).Warn("sync project not started again", "file", f)
			continue
		}
		if _, err := m.Start(string(data), syncproject.NameFromPath(f)); err != nil {
			m.log.WithError(err).Warn("sync project not started again", "file", f)
		}
	}
}

// SetTemplate runs the template's project, nil for none, replacing the one
// running if it changed.
func (m *ProjectManager) SetTemplate(p *syncproject.Project) {
	var enc []byte
	if p != nil {
		var err error
		if enc, err = p.Encode(); err != nil {
			m.log.WithError(err).Warn("the template's sync project is not run")
			return
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if bytes.Equal(enc, m.template) {
		return
	}
	if old := m.runs[syncproject.TemplateProject]; old != nil {
		old.stop()
		delete(m.runs, syncproject.TemplateProject)
	}
	m.template = enc
	if p == nil {
		return
	}
	if err := checkProject(p); err != nil {
		m.log.WithError(err).Warn("the template's sync project is not run")
		return
	}
	if err := m.overlapsLocked(p); err != nil {
		m.log.WithError(err).Warn("the template's sync project is not run")
		return
	}
	m.log.Info("starting the template's sync project", "jobs", len(p.Sync))
	m.runs[p.Name] = startProject(m.ctx, m.client, p, true, m.log, m.follow)
}

// Start runs a project from its file's content, replacing a project of the
// same name, and keeps it to start again. name is its name when the file
// gives none. It returns the project's name.
func (m *ProjectManager) Start(content, name string) (string, error) {
	p, err := syncproject.Parse([]byte(content), name)
	if err != nil {
		return "", err
	}
	if p.Name == syncproject.TemplateProject {
		return "", fmt.Errorf("the name %s is the template's sync project's: give the project another name", syncproject.TemplateProject)
	}
	if err := checkProject(p); err != nil {
		return "", err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	old := m.runs[p.Name]
	delete(m.runs, p.Name)
	if err := m.overlapsLocked(p); err != nil {
		if old != nil {
			m.runs[p.Name] = old
		}
		return "", err
	}
	if old != nil {
		old.stop()
	}
	if err := os.MkdirAll(m.dir, 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(m.dir, p.Name+".toml"), []byte(content), 0o600); err != nil {
		return "", err
	}
	m.log.Info("starting sync project", "project", p.Name, "jobs", len(p.Sync))
	m.runs[p.Name] = startProject(m.ctx, m.client, p, false, m.log, m.follow)
	return p.Name, nil
}

// overlapsLocked refuses a project syncing a directory another running
// project syncs, or one inside it; m.mu is held.
func (m *ProjectManager) overlapsLocked(p *syncproject.Project) error {
	for _, j := range p.Sync {
		mine := filepath.Clean(expandHome(j.Local))
		for name, r := range m.runs {
			for _, o := range r.project.Sync {
				theirs := filepath.Clean(expandHome(o.Local))
				if mine == theirs || strings.HasPrefix(mine, theirs+string(filepath.Separator)) || strings.HasPrefix(theirs, mine+string(filepath.Separator)) {
					return fmt.Errorf("sync job %s syncs %s, which project %s's job %s syncs: stop that first", j.Name, j.Local, name, o.Name)
				}
			}
		}
	}
	return nil
}

// Stop stops a started project and forgets it. The template's keeps
// running.
func (m *ProjectManager) Stop(name string) error {
	if name == syncproject.TemplateProject {
		return fmt.Errorf("the template's sync project runs for as long as the space does")
	}
	m.mu.Lock()
	r := m.runs[name]
	delete(m.runs, name)
	m.mu.Unlock()
	file := filepath.Join(m.dir, name+".toml")
	if r == nil {
		if _, err := os.Stat(file); err != nil {
			return fmt.Errorf("no sync project %s is running", name)
		}
	} else {
		r.stop()
	}
	if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
		return err
	}
	m.log.Info("stopped sync project", "project", name)
	return nil
}

// List is how the running projects are doing, by name.
func (m *ProjectManager) List() []syncproject.ProjectStatus {
	m.mu.Lock()
	runs := make([]*projectRun, 0, len(m.runs))
	for _, r := range m.runs {
		runs = append(runs, r)
	}
	m.mu.Unlock()
	out := make([]syncproject.ProjectStatus, 0, len(runs))
	for _, r := range runs {
		out = append(out, r.status())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Flush has every project send its local changes now, as the space is about
// to stop, waiting until they have or ctx ends.
func (m *ProjectManager) Flush(ctx context.Context) error {
	m.mu.Lock()
	runs := make([]*projectRun, 0, len(m.runs))
	for _, r := range m.runs {
		runs = append(runs, r)
	}
	m.mu.Unlock()
	var wg sync.WaitGroup
	errs := make([]error, len(runs))
	for i, r := range runs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := r.flush(ctx); err != nil {
				errs[i] = fmt.Errorf("sync project %s: %w", r.project.Name, err)
			}
		}()
	}
	wg.Wait()
	err := errors.Join(errs...)
	if err != nil {
		m.log.WithError(err).Warn("local changes may not all have been sent before the space stopped")
	} else if len(runs) > 0 {
		m.log.Info("local changes sent before the space stops")
	}
	return err
}

// StopAll stops every project, as the agent stops, keeping the started
// ones to start again.
func (m *ProjectManager) StopAll() {
	m.mu.Lock()
	runs := m.runs
	m.runs = map[string]*projectRun{}
	m.template = nil
	m.mu.Unlock()
	var wg sync.WaitGroup
	for _, r := range runs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.stop()
		}()
	}
	wg.Wait()
}
