package mcptools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/scriptdocs"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/toolsurface"
	"github.com/paularlott/knot/internal/util/audit"
)

// GoToolHandler implements a built-in tool in Go. ctx is the MCP request's.
type GoToolHandler func(ctx context.Context, params map[string]interface{}, user *model.User) (interface{}, error)

// goToolHandlers are the built-in tools implemented in Go, by name. Each
// still has a TOML beside the scripted tools (with handler = "go") for its
// description and parameters, and can be disabled like them.
var goToolHandlers = map[string]GoToolHandler{
	RunScriptToolName:  runScriptTool,
	ScriptDocsToolName: scriptDocsTool,
}

const (
	// RunScriptToolName runs a script the AI assistant wrote.
	RunScriptToolName = "run_script"
	// ScriptDocsToolName serves the documentation for writing one.
	ScriptDocsToolName = "script_docs"
)

// aiScriptAuditCodeBytes caps the code kept in the audit log, and
// aiScriptAuditErrorBytes the error.
const (
	aiScriptAuditCodeBytes    = 8000
	aiScriptAuditErrorBytes   = 2000
	aiScriptAuditPurposeBytes = 1000
	aiScriptAuditModeBytes    = 40
)

// Surface names where a tool call came from, so a tool can tell whether a
// person approved it. The web assistant asks the user before tool calls and
// shows their arguments; an MCP client is expected to do the same for a tool
// marked destructive. The OpenAI compatible endpoints, scripts calling
// knot.mcp and in-process API calls run tools with no one to ask, and carry
// no surface (in-process calls strip it, see toolsurface.Strip).
type Surface = toolsurface.Surface

const (
	SurfaceWebChat = toolsurface.WebChat
	SurfaceMCP     = toolsurface.MCP
)

// WithSurface marks ctx as coming from a surface that approves tool calls.
func WithSurface(ctx context.Context, surface Surface) context.Context {
	return toolsurface.With(ctx, surface)
}

// SurfaceFrom returns the surface ctx was marked with, "" when none.
func SurfaceFrom(ctx context.Context) Surface {
	return toolsurface.From(ctx)
}

// UserCanRunAIScripts reports whether run_script and script_docs are loaded
// and the user holds the permission to use them.
func UserCanRunAIScripts(user *model.User) bool {
	if user == nil || !user.HasPermission(model.PermissionRunAIScripts) {
		return false
	}
	_, ok := GetTool(RunScriptToolName)
	return ok
}

// AIScriptTool reports whether a tool is one of the AI script tools, which
// only users holding the Run AI Scripts permission are offered.
func AIScriptTool(name string) bool {
	return name == RunScriptToolName || name == ScriptDocsToolName
}

// scriptDocsTool serves script_docs: the guide, or the pages of the named
// libraries.
func scriptDocsTool(ctx context.Context, params map[string]interface{}, user *model.User) (interface{}, error) {
	if err := service.CheckAIScriptPermission(user); err != nil {
		return nil, err
	}
	return scriptdocs.Lookup(stringParam(params, "library"))
}

// runScriptTool serves run_script: checks the permission and the request,
// gets the user's approval for write mode, runs the script and writes the
// audit record.
func runScriptTool(ctx context.Context, params map[string]interface{}, user *model.User) (interface{}, error) {
	if err := service.CheckAIScriptPermission(user); err != nil {
		return nil, err
	}
	raw := service.AIScriptRequest{
		Code:    stringParam(params, "code"),
		Purpose: stringParam(params, "purpose"),
		Mode:    stringParam(params, "mode"),
	}
	req, err := service.NormalizeAIScriptRequest(raw)
	if err != nil {
		// A malformed request is audited as refused, with what it asked
		// for: someone asked for it.
		raw.Mode = truncateUTF8(raw.Mode, aiScriptAuditModeBytes)
		auditAIScript(user, raw, &service.AIScriptRun{Mode: raw.Mode, Error: err.Error()}, SurfaceFrom(ctx), true)
		return nil, err
	}
	if req.Mode == service.AIScriptModeWrite {
		if err := approveWriteScript(ctx, req); err != nil {
			// A refused write is audited too: someone asked for it.
			auditAIScript(user, req, &service.AIScriptRun{Mode: req.Mode, Error: err.Error()}, SurfaceFrom(ctx), true)
			return nil, err
		}
	}

	run, err := runAIScript(ctx, user, req)
	if err != nil {
		// A run that could not start (the concurrency caps) is refused.
		auditAIScript(user, req, &service.AIScriptRun{Mode: req.Mode, Error: err.Error()}, SurfaceFrom(ctx), true)
		return nil, err
	}
	auditAIScript(user, req, run, SurfaceFrom(ctx), false)
	return run, nil
}

// runAIScript and auditAIScriptEntry are variables so tests can run the
// tool without the API mux and the database.
var (
	runAIScript        = service.RunAIScript
	auditAIScriptEntry = audit.Log
)

// errWriteNeedsApproval is returned for write mode where no one can approve it.
var errWriteNeedsApproval = errors.New("write mode needs the user's approval, which cannot be asked for here: " +
	"this call did not come from the knot web assistant or an MCP client. Run the script in read mode, " +
	"or tell the user what to change so they can do it")

// errWriteScopedToken is returned for write mode asked for with an API token
// whose scopes narrow it.
var errWriteScopedToken = errors.New("write mode is not available to a connection signed in with an API token " +
	"limited to particular scopes: a write-mode script can reach far more of the knot API than the token's " +
	"scopes allow. Run the script in read mode, or ask the user to make the change in the knot web assistant " +
	"or web UI")

// approveWriteScript checks a write-mode run was approved by a person. The
// approval is the calling surface's own: run_script is marked destructive,
// so the web assistant asks before every write-mode call (showing the code,
// with no "Always Allow") and MCP clients ask before calling it. knot does
// not ask through elicitation as well: clients that ask for destructive
// tools would then ask twice, and some declare elicitation but decline or
// never show it. Anywhere no one is asked, write mode is refused, and so is
// a request made with an API token limited to scopes: the token's holder was
// given only part of the API, which a write-mode script would step around.
// OAuth tokens are scoped too, but are issued to an interactive MCP client a
// person signed in to, which asks before each destructive call, and the MCP
// tools that token reaches already change spaces, files and templates.
func approveWriteScript(ctx context.Context, req service.AIScriptRequest) error {
	if scopedCredential(ctx) {
		return errWriteScopedToken
	}
	switch SurfaceFrom(ctx) {
	case SurfaceWebChat, SurfaceMCP:
		return nil
	}
	return errWriteNeedsApproval
}

// scopedCredential reports whether the request authenticated with an API
// token whose scopes restrict it (see middleware.ApiAuth). OAuth tokens
// (issued through the authorization-code flow, marked RefreshToken) are not
// counted: see approveWriteScript. Sessions, agent tokens and unscoped API
// tokens have the user's whole API.
func scopedCredential(ctx context.Context) bool {
	token, _ := ctx.Value("access_token").(*model.Token)
	return token != nil && len(token.Scopes) > 0 && !token.RefreshToken
}

// auditAIScript writes one audit log entry for a run, or for a write-mode
// run refused before it started: who, the mode, the purpose, the outcome,
// the call count, the error and the code (both capped). What the script read
// or printed is never logged.
func auditAIScript(user *model.User, req service.AIScriptRequest, run *service.AIScriptRun, surface Surface, refused bool) {
	sum := sha256.Sum256([]byte(req.Code))
	code := req.Code
	if len(code) > aiScriptAuditCodeBytes {
		code = truncateUTF8(code, aiScriptAuditCodeBytes) + "\n[... truncated ...]"
	}
	errText := run.Error
	if len(errText) > aiScriptAuditErrorBytes {
		errText = truncateUTF8(errText, aiScriptAuditErrorBytes) + " [... truncated ...]"
	}
	purpose := req.Purpose
	if len(purpose) > aiScriptAuditPurposeBytes {
		purpose = truncateUTF8(purpose, aiScriptAuditPurposeBytes) + " [... truncated ...]"
	}
	outcome := "ok"
	switch {
	case refused:
		outcome = "refused"
	case !run.OK:
		outcome = "failed"
	}
	properties := map[string]interface{}{
		"mode":        req.Mode,
		"purpose":     purpose,
		"ok":          run.OK,
		"refused":     refused,
		"error":       errText,
		"api_calls":   run.Calls,
		"duration_ms": run.DurationMS,
		"code":        code,
		"code_bytes":  len(req.Code),
		"code_sha256": hex.EncodeToString(sum[:]),
		"surface":     string(surface),
		"user_id":     user.Id,
	}
	details := fmt.Sprintf("Ran an AI-written script in %s mode (%s): %s", req.Mode, outcome, purpose)
	if refused {
		details = fmt.Sprintf("Refused an AI-written script in %q mode: %s", req.Mode, purpose)
	}
	if err := auditAIScriptEntry(user.Username, model.AuditActorTypeUser, model.AuditEventAIScriptRun, details, &properties); err != nil {
		log.WithGroup("mcptools").Warn("could not write the AI script audit log", "user", user.Username, "error", err)
	}
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

// stringParam returns a string argument, "" when absent.
func stringParam(params map[string]interface{}, name string) string {
	switch v := params[name].(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}
