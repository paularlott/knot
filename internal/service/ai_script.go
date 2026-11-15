package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/scriptdocs"
	knotscriptling "github.com/paularlott/knot/internal/scriptling"
	"github.com/paularlott/knot/internal/toolsurface"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/scriptling"
	"github.com/paularlott/scriptling/conversion"
	scriptlingsimilarity "github.com/paularlott/scriptling/extlibs/similarity"
	scriptlingtemplate "github.com/paularlott/scriptling/extlibs/template"
	"github.com/paularlott/scriptling/extlibs/toml"
	"github.com/paularlott/scriptling/extlibs/yaml"
	"github.com/paularlott/scriptling/libloader"
	"github.com/paularlott/scriptling/object"
	"github.com/paularlott/scriptling/stdlib"
)

// Limits of one AI-written script run (the run_script MCP tool).
var (
	AIScriptTimeout        = 60 * time.Second
	AIScriptMaxCalls int64 = 500
	// AIScriptMaxOutput caps the printed output and, separately, the JSON
	// of the `result` variable returned to the model and the error text.
	AIScriptMaxOutput = 50_000
	// AIScriptMemoryLimit caps the interpreter's estimated memory, and
	// AIScriptMaxValueBytes the largest single string, bytes value or
	// container a script may build.
	AIScriptMemoryLimit   int64 = 256 << 20
	AIScriptMaxValueBytes int64 = 8 << 20
	// AIScriptMaxResponseBytes caps the body of one API response (JSON or
	// a file read with get_bytes): the in-process client buffers it whole.
	AIScriptMaxResponseBytes int64 = 16 << 20
	// AIScriptMaxConcurrentPerUser caps the AI script runs one user may
	// have going at once, and AIScriptMaxConcurrent those of everyone on
	// this server: each run may hold its memory limit for its time limit.
	AIScriptMaxConcurrentPerUser = 2
	AIScriptMaxConcurrent        = 8
)

// aiScriptSlots counts the AI script runs in progress, in all and per user.
var aiScriptSlots = struct {
	sync.Mutex
	total  int
	byUser map[string]int
}{byUser: map[string]int{}}

// ErrAIScriptBusy is returned when a run would go over a concurrency cap.
var ErrAIScriptBusy = errors.New("too many AI-written scripts are running")

// acquireAIScriptSlot takes a place for one run by the user, refusing when
// the user already has AIScriptMaxConcurrentPerUser runs going or the
// server AIScriptMaxConcurrent. The release function gives it back.
func acquireAIScriptSlot(userID string) (func(), error) {
	aiScriptSlots.Lock()
	defer aiScriptSlots.Unlock()
	if aiScriptSlots.byUser[userID] >= AIScriptMaxConcurrentPerUser {
		return nil, fmt.Errorf("%w: you already have %d running, the most one user may run at once. Wait for them to finish and try again", ErrAIScriptBusy, AIScriptMaxConcurrentPerUser)
	}
	if aiScriptSlots.total >= AIScriptMaxConcurrent {
		return nil, fmt.Errorf("%w: the server is running its limit of %d at once. Try again in a moment", ErrAIScriptBusy, AIScriptMaxConcurrent)
	}
	aiScriptSlots.total++
	aiScriptSlots.byUser[userID]++
	var once sync.Once
	return func() {
		once.Do(func() {
			aiScriptSlots.Lock()
			defer aiScriptSlots.Unlock()
			aiScriptSlots.total--
			if aiScriptSlots.byUser[userID]--; aiScriptSlots.byUser[userID] <= 0 {
				delete(aiScriptSlots.byUser, userID)
			}
		})
	}, nil
}

// aiScriptLibraries are the knot.* Python libraries an AI-written script may
// import; each reaches knot only through knot.apiclient, which the policy
// guards. Left out: knot.ai and knot.mcp (the model and other tools),
// knot.event (imports requests), knot.secret (secret values), knot.token
// (API tokens) and knot.mcp_server (server configs hold credentials). Every one has a page in internal/scriptdocs, and every page
// there names one of these or a Go library registered below
// (TestAIScriptLibrariesMatchDocs).
var aiScriptLibraries = []string{
	"knot.audit", "knot.event_sink", "knot.files", "knot.group", "knot.jobs",
	"knot.permission", "knot.pool", "knot.role", "knot.script", "knot.server",
	"knot.skill", "knot.slash_command", "knot.space", "knot.stack",
	"knot.template", "knot.tunnel", "knot.user", "knot.vars", "knot.volume",
}

// aiScriptGoLibraries are registered in Go rather than loaded: the guarded
// API transport and the user the script runs as.
var aiScriptGoLibraries = []string{"knot.apiclient", "knot.identity"}

// aiScriptExcludedStdlib are standard modules an AI script does not get:
// secrets (randomness for credentials has no use here) and html.parser.
var aiScriptExcludedStdlib = map[string]bool{
	stdlib.SecretsLibraryName:    true,
	stdlib.HTMLParserLibraryName: true,
}

// aiScriptPreludeModules are the standard modules imported before every script.
var aiScriptPreludeModules = []string{"datetime", "json", "math", "re", "statistics"}

// aiScriptAliases renames a library's short name where it would hide a
// builtin.
var aiScriptAliases = map[string]string{"knot.vars": "variables"}

// AIScriptRequest is one run_script call.
type AIScriptRequest struct {
	Code    string
	Purpose string
	Mode    string // AIScriptModeRead (default) or AIScriptModeWrite
}

// AIScriptRun is the outcome of one AI script run, as returned to the model.
type AIScriptRun struct {
	OK         bool   `json:"ok" msgpack:"ok"`
	Output     string `json:"output" msgpack:"output"`
	Result     any    `json:"result,omitempty" msgpack:"result,omitempty"`
	Error      string `json:"error,omitempty" msgpack:"error,omitempty"`
	Truncated  bool   `json:"truncated,omitempty" msgpack:"truncated,omitempty"`
	Note       string `json:"note,omitempty" msgpack:"note,omitempty"`
	Calls      int64  `json:"calls" msgpack:"calls"`
	Mode       string `json:"mode" msgpack:"mode"`
	DurationMS int64  `json:"duration_ms" msgpack:"duration_ms"`
}

// ErrAIScriptPermission is returned when the user may not run AI scripts.
var ErrAIScriptPermission = errors.New("you don't have permission to run AI scripts: an administrator must give you a role with the \"Run AI Scripts\" permission")

// NormalizeAIScriptRequest checks and fills in a request: code and purpose
// are required and the mode defaults to read. The mode must be exactly
// "read" or "write" (or absent): the web assistant decides whether to ask
// the user by matching the argument as given, so a variant such as "WRITE"
// must not be taken as write mode.
func NormalizeAIScriptRequest(req AIScriptRequest) (AIScriptRequest, error) {
	req.Purpose = strings.TrimSpace(req.Purpose)
	if strings.TrimSpace(req.Code) == "" {
		return req, errors.New("code is required: write the script, and call script_docs for how")
	}
	if req.Purpose == "" {
		return req, errors.New("purpose is required: say in one sentence what the script works out or changes")
	}
	switch req.Mode {
	case "":
		req.Mode = AIScriptModeRead
	case AIScriptModeRead, AIScriptModeWrite:
	default:
		return req, fmt.Errorf("mode must be exactly \"read\" or \"write\" (lower case, no spaces), not %q", truncateUTF8(req.Mode, 40))
	}
	return req, nil
}

// CheckAIScriptPermission returns ErrAIScriptPermission unless the user
// holds the Run AI Scripts permission.
func CheckAIScriptPermission(user *model.User) error {
	if user == nil || !user.HasPermission(model.PermissionRunAIScripts) {
		return ErrAIScriptPermission
	}
	return nil
}

// RunAIScript runs a script the AI assistant wrote, as the user, through
// their own API client wrapped in the AI script policy. The caller has
// already obtained any approval write mode needs, and writes the audit
// record. Problems in the script itself come back in the run (ok false) so
// the model can fix and retry; the error is for a run that could not start.
func RunAIScript(ctx context.Context, user *model.User, req AIScriptRequest) (*AIScriptRun, error) {
	if err := CheckAIScriptPermission(user); err != nil {
		return nil, err
	}
	req, err := NormalizeAIScriptRequest(req)
	if err != nil {
		return nil, err
	}
	release, err := acquireAIScriptSlot(user.Id)
	if err != nil {
		return nil, err
	}
	defer release()
	client := apiclient.NewMuxClient(user).GetRESTClient()
	return runAIScript(ctx, user, client, req), nil
}

// runAIScript runs the script over the given client (the user's own, or a
// test double), guarded by a fresh policy.
func runAIScript(ctx context.Context, user *model.User, client rest.RESTClient, req AIScriptRequest) *AIScriptRun {
	policy := &AIScriptPolicy{Mode: req.Mode, MaxCalls: AIScriptMaxCalls}
	started := time.Now()

	// The run's context is the one every API call the script makes is
	// served under: it ends the in-process requests at the time limit,
	// bounds each response, and carries no tool-call surface, so nothing the
	// script calls can count as approved by the person who approved it.
	runCtx, cancel := context.WithTimeout(context.WithValue(toolsurface.Strip(ctx), "user", user), AIScriptTimeout)
	defer cancel()
	runCtx = rest.WithResponseLimit(runCtx, AIScriptMaxResponseBytes)

	output := &cappedWriter{limit: AIScriptMaxOutput}
	env := newAIScriptEnv(newAIScriptClient(client, policy), user, output)
	defer env.Close()

	prelude := aiScriptPrelude()
	_, evalErr := env.EvalWithContext(runCtx, prelude+req.Code)

	run := &AIScriptRun{OK: evalErr == nil, Mode: req.Mode}
	run.Output, run.Truncated = output.String()
	if evalErr != nil {
		switch {
		case errors.Is(runCtx.Err(), context.DeadlineExceeded):
			run.Error = fmt.Sprintf("the script exceeded the %s time limit: make fewer API calls, or narrow the question", AIScriptTimeout)
		case errors.Is(evalErr, scriptling.ErrMemoryLimitExceeded) || strings.Contains(evalErr.Error(), scriptling.ErrMemoryLimitExceeded.Error()):
			run.Error = fmt.Sprintf("the script went over its %d MB memory limit: work on fewer rows at a time, or keep less of what it reads", AIScriptMemoryLimit>>20)
		default:
			run.Error = scriptLines(evalErr.Error(), strings.Count(prelude, "\n"))
		}
		if len(run.Error) > AIScriptMaxOutput {
			run.Error = truncateUTF8(run.Error, AIScriptMaxOutput) + "\n[... error truncated at " + fmt.Sprint(AIScriptMaxOutput) + " bytes ...]"
			run.Truncated = true
		}
	} else if obj, getErr := env.GetVarAsObject("result"); getErr == nil && obj != nil {
		// Eval has returned, so bind the script's guard: the size check and
		// the conversion below are then held to its memory and value limits.
		releaseGuard := object.BindGuard(env.ResourceGuard())
		defer releaseGuard()
		if _, isNull := obj.(*object.Null); isNull {
			// No result.
		} else if size := knotscriptling.JSONSizeAtLeast(obj, AIScriptMaxOutput); size > AIScriptMaxOutput {
			// Refused before it is converted: a result many times the cap
			// would otherwise be copied and encoded whole first.
			run.OK = false
			run.Truncated = true
			run.Error = fmt.Sprintf("result is over the %d byte limit: aggregate it, or return fewer rows", AIScriptMaxOutput)
		} else {
			value, convErr := conversion.ToGoLimited(obj)
			if convErr != nil {
				run.OK = false
				run.Error = "result could not be returned: " + convErr.Message
			} else if encoded, jsonErr := json.Marshal(value); jsonErr != nil {
				run.OK = false
				run.Error = "result could not be returned as JSON: " + jsonErr.Error()
			} else if len(encoded) > AIScriptMaxOutput {
				run.OK = false
				run.Truncated = true
				run.Error = fmt.Sprintf("result is %d bytes, over the %d byte limit: aggregate it, or return fewer rows", len(encoded), AIScriptMaxOutput)
			} else {
				run.Result = value
			}
		}
	}
	if run.OK && run.Result == nil && strings.TrimSpace(run.Output) == "" {
		run.Note = "The script ran but returned nothing: set a variable named result, or print the answer."
	}
	run.Calls = policy.Calls()
	run.DurationMS = time.Since(started).Milliseconds()
	return run
}

// newAIScriptEnv builds the environment an AI-written script runs in: the
// standard modules (less aiScriptExcludedStdlib), the pure data libraries
// (yaml, toml, templates, string similarity), knot.apiclient over the
// guarded client, knot.identity, docs(), and a loader that only finds the
// allowed knot.* libraries. Deliberately absent, unlike the MCP tool
// environment (NewServerScriptlingEnv): requests, wait_for, scriptling.ai
// and scriptling.mcp (no network), fs and the other system libraries,
// logging (what a script read must not land in the server log), platform,
// knot.ai, knot.mcp, knot.plugin.call, plugin exports, and the user's own
// lib scripts from the server.
func newAIScriptEnv(client rest.RESTClient, user *model.User, output *cappedWriter) *scriptling.Scriptling {
	env := scriptling.New(
		scriptling.WithMemoryLimit(AIScriptMemoryLimit),
		scriptling.WithMaxValueBytes(AIScriptMaxValueBytes),
	)
	env.SetOutputWriter(output)

	stdlib.RegisterAll(filteredRegistrar{env, aiScriptExcludedStdlib})
	yaml.Register(env)
	toml.Register(env)
	scriptlingsimilarity.Register(env)
	scriptlingtemplate.RegisterText(env)

	userID := ""
	if user != nil {
		userID = user.Id
		env.RegisterLibrary(knotscriptling.GetIdentityLibrary(NewUserObject(user)))
	}
	env.RegisterLibrary(knotscriptling.GetApiClientLibrary(client, userID))

	env.RegisterFunc("docs", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		name := ""
		if len(args) > 0 {
			name, _ = args[0].AsString()
		}
		if strings.TrimSpace(name) == "" {
			return object.NewString(scriptdocs.Guide())
		}
		page, err := scriptdocs.Library(name)
		if err != nil {
			return &object.Error{Message: err.Error()}
		}
		return object.NewString(page)
	}, `docs(name="") - Documentation for AI scripts

  With no name: the guide to writing scripts. With a library name such as
  "knot.space" (or "space"): that library's page.`)

	allowed := map[string]bool{}
	for _, name := range aiScriptLibraries {
		allowed[name] = true
	}
	knotLibs := newKnotLibsLoader()
	env.SetLibraryLoader(libloader.NewFuncLoader(func(name string) (string, bool, error) {
		if !allowed[name] {
			return "", false, nil
		}
		return knotLibs.Load(name)
	}, "ai-script-knot-libs"))
	return env
}

// filteredRegistrar registers every library except the excluded ones.
type filteredRegistrar struct {
	p        interface{ RegisterLibrary(*object.Library) }
	excluded map[string]bool
}

func (f filteredRegistrar) RegisterLibrary(lib *object.Library) {
	if !f.excluded[lib.Name()] {
		f.p.RegisterLibrary(lib)
	}
}

// aiScriptPrelude imports the common standard modules, defaultdict and every
// allowed knot library under its short name (knot.space as space), so a
// script needs no imports. It runs as one program with the script: the
// script's line numbers are recovered with scriptLines.
func aiScriptPrelude() string {
	var b strings.Builder
	for _, module := range aiScriptPreludeModules {
		b.WriteString("import " + module + "\n")
	}
	b.WriteString("from collections import defaultdict\n")
	for _, name := range append(append([]string{}, aiScriptGoLibraries...), aiScriptLibraries...) {
		b.WriteString("import " + name + " as " + aiScriptShortName(name) + "\n")
	}
	return b.String()
}

// aiScriptShortName is the name the prelude imports a library under.
func aiScriptShortName(name string) string {
	if alias, ok := aiScriptAliases[name]; ok {
		return alias
	}
	return strings.TrimPrefix(name, "knot.")
}

var errorLine = regexp.MustCompile(`line (\d+)`)

// scriptLines renumbers the line numbers in an error from the
// prelude-prefixed program to the script's own lines.
func scriptLines(message string, preludeLines int) string {
	return errorLine.ReplaceAllStringFunc(message, func(match string) string {
		var n int
		if _, err := fmt.Sscanf(match, "line %d", &n); err != nil {
			return match
		}
		if n <= preludeLines {
			return fmt.Sprintf("line %d of the prelude", n)
		}
		return fmt.Sprintf("line %d", n-preludeLines)
	})
}

// cappedWriter keeps the first limit bytes written to it and drops the rest,
// so a script printing in a loop cannot grow the server's memory.
type cappedWriter struct {
	mu      sync.Mutex
	limit   int
	buf     strings.Builder
	dropped bool
}

func (w *cappedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	room := w.limit - w.buf.Len()
	if room <= 0 {
		w.dropped = w.dropped || len(p) > 0
		return len(p), nil
	}
	if len(p) > room {
		w.buf.Write(p[:room])
		w.dropped = true
		return len(p), nil
	}
	w.buf.Write(p)
	return len(p), nil
}

// String returns what was kept and whether anything was dropped.
func (w *cappedWriter) String() (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.dropped {
		return w.buf.String() + "\n[... output truncated at " + fmt.Sprint(w.limit) + " bytes ...]", true
	}
	return w.buf.String(), false
}

// truncateUTF8 cuts s to at most n bytes without splitting a character.
func truncateUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && n < len(s) && (s[n]&0xC0) == 0x80 {
		n--
	}
	return s[:n]
}
