package scriptling

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/paularlott/knot/internal/syncproject"
	"github.com/paularlott/scriptling/conversion"
	"github.com/paularlott/scriptling/errors"
	"github.com/paularlott/scriptling/object"
)

// SyncProjectAgent runs a space's sync projects: the agent's own manager
// for scripts the agent runs, its command socket for knot run-script.
type SyncProjectAgent interface {
	// Start runs a project from its TOML, named name when it gives no
	// name, and returns its name.
	Start(content, name string) (string, error)
	Stop(name string) error
	List() ([]syncproject.ProjectStatus, error)
}

var (
	syncAgentMu    sync.Mutex
	syncAgent      SyncProjectAgent
	syncAgentReady = make(chan struct{}) // closed while syncAgent is set
	syncAgentDue   atomic.Bool
)

// syncAgentWait is how long a script waits for the agent's sync projects to
// start, a startup script running as the agent connects.
const syncAgentWait = time.Minute

// ExpectSyncProjectAgent says the process will install a SyncProjectAgent
// shortly: the agent, which runs startup scripts as it connects, before its
// sync projects have started. knot.files.sync waits for it then.
func ExpectSyncProjectAgent() { syncAgentDue.Store(true) }

// SetSyncProjectAgent installs what knot.files.sync manages sync projects
// through; nil takes it away, as the agent stops.
func SetSyncProjectAgent(a SyncProjectAgent) {
	syncAgentMu.Lock()
	defer syncAgentMu.Unlock()
	was := syncAgent
	syncAgent = a
	switch {
	case a != nil && was == nil:
		close(syncAgentReady)
	case a == nil && was != nil:
		syncAgentReady = make(chan struct{})
	}
}

func currentSyncAgent(ctx context.Context) (SyncProjectAgent, error) {
	syncAgentMu.Lock()
	a, ready := syncAgent, syncAgentReady
	syncAgentMu.Unlock()
	if a != nil {
		return a, nil
	}
	if syncAgentDue.Load() {
		t := time.NewTimer(syncAgentWait)
		defer t.Stop()
		select {
		case <-ready:
		case <-t.C:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		syncAgentMu.Lock()
		a = syncAgent
		syncAgentMu.Unlock()
		if a != nil {
			return a, nil
		}
	}
	return nil, fmt.Errorf("no knot agent is running here: sync projects run in a space's agent")
}

// syncOutsideSpace is the error knot.files.sync gives where there is no
// space's agent to run sync projects: server-side scripts.
const syncOutsideSpace = "knot.files.sync manages the sync projects a space's agent runs: use it in a script that runs in a space"

// syncWaitDefault is how long start(wait=True) waits for a project's first
// passes when no timeout is given.
const syncWaitDefault = 10 * time.Minute

// GetSyncLibrary returns knot.files.sync, which starts, stops and reports
// on the sync projects a space's agent runs. inSpace is false for scripts
// that run elsewhere, where every function refuses.
func GetSyncLibrary(inSpace bool) *object.Library {
	builder := object.NewLibraryBuilder("knot.files.sync", "Run sync projects in a space: keep folders in step with file storage")

	refuse := func() object.Object { return &object.Error{Message: syncOutsideSpace} }
	agentFor := func(ctx context.Context) (SyncProjectAgent, object.Object) {
		if !inSpace {
			return nil, refuse()
		}
		a, err := currentSyncAgent(ctx)
		if err != nil {
			return nil, &object.Error{Message: err.Error()}
		}
		return a, nil
	}

	builder.FunctionWithHelp("start", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		if err := errors.MinArgs(args, 1); err != nil {
			return err
		}
		content, errObj := projectContent(args[0])
		if errObj != nil {
			return errObj
		}
		name, errObj := kwargs.GetString("name", "")
		if errObj != nil {
			return errObj
		}
		wait, errObj := kwargs.GetBool("wait", false)
		if errObj != nil {
			return errObj
		}
		timeout, errObj := kwargs.GetFloat("timeout", syncWaitDefault.Seconds())
		if errObj != nil {
			return errObj
		}
		if timeout <= 0 {
			return &object.Error{Message: "timeout must be more than 0 seconds"}
		}
		// Checked here too, so a mistake reads the same in and out of a space.
		p, err := syncproject.Parse([]byte(content), name)
		if err != nil {
			return &object.Error{Message: err.Error()}
		}
		a, errObj := agentFor(ctx)
		if errObj != nil {
			return errObj
		}
		started, err := a.Start(content, p.Name)
		if err != nil {
			return &object.Error{Message: err.Error()}
		}
		if !wait {
			st, err := projectStatus(a, started)
			if err != nil {
				return &object.Error{Message: err.Error()}
			}
			return statusObject(st)
		}
		wctx, cancel := context.WithTimeout(ctx, time.Duration(timeout*float64(time.Second)))
		defer cancel()
		for {
			st, err := projectStatus(a, started)
			if err != nil {
				return &object.Error{Message: err.Error()}
			}
			if st == nil {
				return &object.Error{Message: fmt.Sprintf("sync project %s is no longer running", started)}
			}
			if settled, failed := st.Settled(); settled {
				if failed {
					return &object.Error{Message: fmt.Sprintf("sync project %s: %s", started, failures(st))}
				}
				return statusObject(st)
			}
			select {
			case <-wctx.Done():
				if ctx.Err() != nil {
					return &object.Error{Message: ctx.Err().Error()}
				}
				return &object.Error{Message: fmt.Sprintf("sync project %s did not finish its first passes within %g seconds; it carries on", started, timeout)}
			case <-time.After(500 * time.Millisecond):
			}
		}
	}, `start(project, name="", wait=False, timeout=600) - Start a sync project in this space's agent, or replace one of the same name; one already running unchanged is left running. project is a dict like a project file ({"name": ..., "sync": [{"local": ..., "remote": "bucket:folder/"}]}) or a project file's TOML; name names a project that gives none. With wait, waits up to timeout seconds for every job's first pass, raising if one fails. Returns the project's status, as status()`)

	builder.FunctionWithHelp("stop", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		if err := errors.ExactArgs(args, 1); err != nil {
			return err
		}
		name, err := args[0].AsString()
		if err != nil {
			return errors.ParameterError("name", err)
		}
		a, errObj := agentFor(ctx)
		if errObj != nil {
			return errObj
		}
		if err := a.Stop(name); err != nil {
			return &object.Error{Message: err.Error()}
		}
		return object.NewBoolean(true)
	}, `stop(name) - Stop a sync project after sending its last local changes, and forget it; the template's project cannot be stopped. Returns True`)

	builder.FunctionWithHelp("list", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		a, errObj := agentFor(ctx)
		if errObj != nil {
			return errObj
		}
		ps, err := a.List()
		if err != nil {
			return &object.Error{Message: err.Error()}
		}
		out := make([]interface{}, 0, len(ps))
		for i := range ps {
			out = append(out, statusMap(&ps[i]))
		}
		return toObject(out)
	}, `list() - The sync projects running in this space, the template's included, each as status() returns it`)

	builder.FunctionWithHelp("status", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		if err := errors.ExactArgs(args, 1); err != nil {
			return err
		}
		name, err := args[0].AsString()
		if err != nil {
			return errors.ParameterError("name", err)
		}
		a, errObj := agentFor(ctx)
		if errObj != nil {
			return errObj
		}
		st, serr := projectStatus(a, name)
		if serr != nil {
			return &object.Error{Message: serr.Error()}
		}
		if st == nil {
			return &object.Null{}
		}
		return statusObject(st)
	}, `status(name) - How a sync project is doing, or None when none of that name runs: a dict with name, template (True for the template's) and jobs, each with name, local, remote, mode, state (starting, snapshotting, syncing, watching, done, failed or stopped), passes, last_pass, error, uploaded, downloaded, deleted and conflicts`)

	return builder.Build()
}

// projectContent is a project's TOML from a dict like a project file, or a
// project file's TOML as given.
func projectContent(arg object.Object) (string, object.Object) {
	if _, ok := arg.(*object.String); ok {
		s, err := arg.AsString()
		if err != nil {
			return "", errors.ParameterError("project", err)
		}
		return s, nil
	}
	v, errObj := conversion.ToGoWithError(arg)
	if errObj != nil {
		return "", errObj
	}
	if _, ok := v.(map[string]interface{}); !ok {
		return "", &object.Error{Message: "project must be a dict like a project file, or a project file's TOML"}
	}
	data, err := json.Marshal(v)
	if err != nil {
		return "", &object.Error{Message: err.Error()}
	}
	var p syncproject.Project
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return "", &object.Error{Message: "project: " + err.Error()}
	}
	out, err := p.Encode()
	if err != nil {
		return "", &object.Error{Message: err.Error()}
	}
	return string(out), nil
}

func projectStatus(a SyncProjectAgent, name string) (*syncproject.ProjectStatus, error) {
	ps, err := a.List()
	if err != nil {
		return nil, err
	}
	for i := range ps {
		if ps[i].Name == name {
			return &ps[i], nil
		}
	}
	return nil, nil
}

// failures says which of a project's jobs failed, and why.
func failures(st *syncproject.ProjectStatus) string {
	var b bytes.Buffer
	for _, j := range st.Jobs {
		if j.State != "failed" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("; ")
		}
		fmt.Fprintf(&b, "job %s failed: %s", j.Name, j.Error)
	}
	return b.String()
}

func statusObject(st *syncproject.ProjectStatus) object.Object {
	return toObject(statusMap(st))
}

// statusMap is a project's status with every key there, empty or not.
func statusMap(st *syncproject.ProjectStatus) map[string]interface{} {
	jobs := make([]interface{}, 0, len(st.Jobs))
	for _, j := range st.Jobs {
		jobs = append(jobs, map[string]interface{}{
			"name": j.Name, "local": j.Local, "remote": j.Remote, "mode": j.Mode,
			"state": j.State, "passes": j.Passes, "last_pass": j.LastPass, "error": j.Error,
			"uploaded": j.Uploaded, "downloaded": j.Downloaded, "deleted": j.Deleted, "conflicts": j.Conflicts,
		})
	}
	return map[string]interface{}{"name": st.Name, "template": st.Template, "jobs": jobs}
}

// toObject converts through JSON, so numbers stay integers.
func toObject(v interface{}) object.Object {
	data, err := json.Marshal(v)
	if err != nil {
		return &object.Error{Message: err.Error()}
	}
	o, err := conversion.ParseJSON(string(data))
	if err != nil {
		return &object.Error{Message: err.Error()}
	}
	return o
}
