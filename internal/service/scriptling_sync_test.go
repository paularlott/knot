package service

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	knotscriptling "github.com/paularlott/knot/internal/scriptling"
	"github.com/paularlott/knot/internal/syncproject"
	"github.com/paularlott/logger"
)

// fakeSyncAgent runs sync projects as an agent would report them: each
// job starting, then past its first pass on the next look, or failed when
// its remote's bucket is "missing".
type fakeSyncAgent struct {
	mu       sync.Mutex
	projects map[string]*syncproject.Project
	content  map[string]string
	looks    map[string]int
	starts   int
}

func newFakeSyncAgent() *fakeSyncAgent {
	return &fakeSyncAgent{projects: map[string]*syncproject.Project{}, content: map[string]string{}, looks: map[string]int{}}
}

func (f *fakeSyncAgent) Start(content, name string) (string, error) {
	p, err := syncproject.Parse([]byte(content), name)
	if err != nil {
		return "", err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.starts++
	f.projects[p.Name], f.content[p.Name], f.looks[p.Name] = p, content, 0
	return p.Name, nil
}

func (f *fakeSyncAgent) Stop(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.projects[name] == nil {
		return fmt.Errorf("no sync project %s is running", name)
	}
	delete(f.projects, name)
	return nil
}

func (f *fakeSyncAgent) List() ([]syncproject.ProjectStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []syncproject.ProjectStatus
	for name, p := range f.projects {
		f.looks[name]++
		st := syncproject.ProjectStatus{Name: name}
		for _, j := range p.Sync {
			js := syncproject.JobStatus{Name: j.Name, Local: j.Local, Remote: j.Remote, Mode: j.SyncMode(), State: "starting"}
			switch {
			case strings.HasPrefix(j.Remote, "missing:"):
				js.State, js.Error = "failed", "bucket not found"
			case f.looks[name] > 1:
				js.State, js.Passes, js.Uploaded = "watching", 1, 3
			}
			st.Jobs = append(st.Jobs, js)
		}
		out = append(out, st)
	}
	return out, nil
}

func agentEnvEval(t *testing.T, script string) (string, error) {
	t.Helper()
	env, cleanup, err := NewAgentScriptlingEnv(nil, "", AgentScriptlingOptions{Logger: logger.NewNullLogger(), Output: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if _, err := env.Eval(script); err != nil {
		return "", err
	}
	v, errObj := env.GetVar("result")
	if errObj != nil {
		t.Fatalf("script set no result: %v", errObj)
	}
	return fmt.Sprint(v), nil
}

func TestKnotFilesSyncLibrary(t *testing.T) {
	agent := newFakeSyncAgent()
	knotscriptling.SetSyncProjectAgent(agent)
	defer knotscriptling.SetSyncProjectAgent(nil)

	out, err := agentEnvEval(t, `
import knot.files.sync as sync

result = []
st = sync.start({"name": "code", "exclude": ["*.log"], "sync": [
    {"local": "~/src", "remote": "code:src/", "snapshot": "code:starter/"},
    {"name": "git", "local": "~/repo", "remote": "code:repo/", "no_default_excludes": True},
]}, wait=True)
result.append(st["name"])
result.append([j["name"] + "=" + j["state"] for j in st["jobs"]])
result.append(st["jobs"][0]["uploaded"] + 1)
result.append(st["jobs"][0]["error"])
result.append(st["template"])

toml = """
[[sync]]
local = "~/notes"
remote = "code:notes/"
mode = "upload"
"""
st = sync.start(toml, name="notes")
result.append(st["jobs"][0]["mode"])
result.append(sorted([p["name"] for p in sync.list()]))
result.append(sync.status("nothing") == None)
result.append(sync.stop("notes"))
result.append(sorted([p["name"] for p in sync.list()]))
`)
	if err != nil {
		t.Fatal(err)
	}
	want := "[code [src=watching git=watching] 4  false upload [code notes] true true [code]]"
	if out != want {
		t.Errorf("got  %s\nwant %s", out, want)
	}
	agent.mu.Lock()
	content := agent.content["code"]
	agent.mu.Unlock()
	for _, s := range []string{`snapshot = "code:starter/"`, `no_default_excludes = true`, `exclude = ["*.log"]`} {
		if !strings.Contains(content, s) {
			t.Errorf("the project the agent got lacks %s:\n%s", s, content)
		}
	}

	// The mistakes a script makes, said plainly.
	out, err = agentEnvEval(t, `
import knot.files.sync as sync

result = {}
for name, fn in [
    ("unknown_key", lambda: sync.start({"name": "x", "sync": [{"local": "~/a", "remote": "code:a/", "wtach": False}]})),
    ("no_name", lambda: sync.start("[[sync]]\nlocal = \"~/a\"\nremote = \"code:a/\"\n")),
    ("bad_remote", lambda: sync.start({"name": "x", "sync": [{"local": "~/a", "remote": "nobucket"}]})),
    ("not_a_project", lambda: sync.start(["x"])),
    ("job_failed", lambda: sync.start({"name": "broken", "sync": [{"local": "~/b", "remote": "missing:b/"}]}, wait=True)),
    ("stop_unknown", lambda: sync.stop("nope")),
]:
    try:
        fn()
        result[name] = ""
    except Exception as e:
        result[name] = str(e)
`)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"unknown field", "needs a name", "bucket:folder", "dict like a project file", "job b failed: bucket not found", "no sync project nope"} {
		if !strings.Contains(out, want) {
			t.Errorf("no error saying %q in %s", want, out)
		}
	}

	// The other ways to import it, beside knot.files.
	for _, script := range []string{
		"import knot.files\nimport knot.files.sync\nresult = len(knot.files.sync.list())",
		"from knot.files import sync\nresult = len(sync.list())",
		"import knot.files.sync as s\nimport knot.files as files\nresult = len(s.list()) + (1 if files.read_text else 0) - 1",
	} {
		if out, err := agentEnvEval(t, script); err != nil || out != "2" {
			t.Errorf("%q: %v %v", script, out, err)
		}
	}
}

// A startup script can run as the agent connects, before its sync projects
// start: it waits for them rather than failing.
func TestKnotFilesSyncWaitsForTheAgent(t *testing.T) {
	knotscriptling.SetSyncProjectAgent(nil)
	knotscriptling.ExpectSyncProjectAgent()
	agent := newFakeSyncAgent()
	go func() {
		time.Sleep(300 * time.Millisecond)
		knotscriptling.SetSyncProjectAgent(agent)
	}()
	defer knotscriptling.SetSyncProjectAgent(nil)
	out, err := agentEnvEval(t, `
import knot.files.sync as sync
result = sync.start({"name": "early", "sync": [{"local": "~/e", "remote": "code:e/"}]})["name"]
`)
	if err != nil || out != "early" {
		t.Fatalf("%v %v", out, err)
	}
}
