package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/plugins"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/scriptling/conversion"
)

// pluginLogoLight and pluginLogoDark return the plugin-provided site-logo
// URLs for the template data, or "" when no plugin claims the site logo or
// an explicit server.ui.logo_url config is set — admin config beats plugins,
// which beat the built-in knot logo.
func pluginLogoLight(cfg *config.ServerConfig) string {
	if cfg.UI.LogoURL != "" {
		return ""
	}
	if registry := plugins.GetRegistry(); registry != nil {
		light, _, _ := registry.SiteLogoURLs()
		return light
	}
	return ""
}

func pluginLogoDark(cfg *config.ServerConfig) string {
	if cfg.UI.LogoURL != "" {
		return ""
	}
	if registry := plugins.GetRegistry(); registry != nil {
		_, dark, _ := registry.SiteLogoURLs()
		return dark
	}
	return ""
}

// pluginAssetTypes maps the file extensions a plugin may declare for its
// logos to the content type served. Anything else is refused — assets are a
// fixed, declared surface, not a general file server.
var pluginAssetTypes = map[string]string{
	".svg":  "image/svg+xml",
	".png":  "image/png",
	".webp": "image/webp",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
}

// HandlePluginAsset serves a plugin's declared assets (the logos from
// [tool.knot]). Only paths a plugin declared at load time are reachable:
// the requested path is matched against the loaded plugin's LogoLight /
// LogoDark exactly, so nothing else in the plugin folder — or the server —
// is exposed by this handler.
func HandlePluginAsset(w http.ResponseWriter, r *http.Request) {
	pluginName := r.PathValue("plugin_name")
	assetPath := r.PathValue("asset_path")

	registry := plugins.GetRegistry()
	if registry == nil {
		http.NotFound(w, r)
		return
	}

	var plugin *plugins.Plugin
	for _, p := range registry.All() {
		if p.Name == pluginName {
			plugin = p
			break
		}
	}
	if plugin == nil {
		http.NotFound(w, r)
		return
	}

	// Only the declared logos are servable.
	if assetPath != plugin.LogoLight && assetPath != plugin.LogoDark {
		http.NotFound(w, r)
		return
	}

	contentType, ok := pluginAssetTypes[filepath.Ext(assetPath)]
	if !ok {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, max-age=300")
	http.ServeFile(w, r, filepath.Join(plugin.Dir, filepath.FromSlash(assetPath)))
}

// pluginPageTimeout bounds one handler dispatch; falls back to 30s when no
// MCP tool timeout is configured.
func pluginPageTimeout() time.Duration {
	if cfg := config.GetServerConfig(); cfg != nil && cfg.MCPToolTimeout > 0 {
		return time.Duration(cfg.MCPToolTimeout) * time.Second
	}
	return 30 * time.Second
}

// HandlePluginPage dispatches a declared plugin page: knot checks the gate,
// then evaluates the plugin's entry file in a fresh run-as-user environment
// and calls the declared handler, rendering its returned value. The
// environment (and everything in it) is discarded with the request — plugin
// code runs only here, never at boot.
func HandlePluginPage(w http.ResponseWriter, r *http.Request) {
	pluginName := r.PathValue("plugin_name")
	pagePath := "/" + r.PathValue("path")

	registry := plugins.GetRegistry()
	if registry == nil {
		http.NotFound(w, r)
		return
	}
	plugin, page := registry.Page(pluginName, pagePath)
	if plugin == nil || page == nil {
		http.NotFound(w, r)
		return
	}

	user := r.Context().Value("user").(*model.User)

	// knot enforces the gate — a plugin cannot forget or skip it.
	if page.Permission != "" && !user.HasPluginPermission(page.Permission) {
		showPageForbidden(w, r)
		return
	}
	if page.Group != "" {
		groups := []string{page.Group}
		if !user.HasAnyGroup(&groups) {
			showPageForbidden(w, r)
			return
		}
	}

	client := apiclient.NewMuxClient(user)
	env, err := service.NewPluginScriptlingEnv(client, user, plugin)
	if err != nil {
		log.Error("plugin page: env", "plugin", plugin.Name, "error", err)
		renderPluginPageError(w, r, plugin, page, "failed to build the plugin environment")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), pluginPageTimeout())
	defer cancel()

	if _, err := env.EvalWithContext(ctx, plugin.EntrySource); err != nil {
		log.Error("plugin page: entry", "plugin", plugin.Name, "error", err)
		renderPluginPageError(w, r, plugin, page, fmt.Sprintf("plugin entry failed: %v", err))
		return
	}

	// The request reaches the handler as the params dict: query parameters,
	// plus — for actions — a POST body (form-encoded or JSON), which wins on
	// key conflicts. knot's own control parameters stay out of params, but
	// _data and _block pass through so handlers can serve dynamic option
	// fetches and cheap per-block refreshes.
	params, err := pluginParams(r)
	if err != nil {
		renderPluginPageError(w, r, plugin, page, "invalid request body")
		return
	}
	if err := env.SetObjectVar("request", conversion.FromGo(map[string]any{"method": r.Method, "path": r.URL.Path})); err != nil {
		renderPluginPageError(w, r, plugin, page, "failed to set the request object")
		return
	}
	if err := env.SetObjectVar("params", conversion.FromGo(params)); err != nil {
		log.Error("plugin page: params", "plugin", plugin.Name, "error", err)
		renderPluginPageError(w, r, plugin, page, "failed to set handler params")
		return
	}

	result, err := env.CallFunctionWithContext(ctx, page.Handler)
	if err != nil {
		log.Error("plugin page: handler", "plugin", plugin.Name, "handler", page.Handler, "error", err)
		renderPluginPageError(w, r, plugin, page, fmt.Sprintf("handler %q failed: %v", page.Handler, err))
		return
	}

	title := page.Label
	if title == "" {
		title = plugin.Name + page.Path
	}

	value := conversion.ToGo(result)

	if handler := r.URL.Query().Get("_col"); handler != "" {
		// Proxy straight to the column's data handler: the page gate was
		// checked above, the environment is jailed and bound to the
		// requesting user, and the handler name came from the layout the
		// same user was served - no layout re-resolution per fetch.
		colResult, err := env.CallFunctionWithContext(ctx, handler)
		if err != nil {
			log.Error("plugin column", "plugin", plugin.Name, "handler", handler, "error", err)
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"column handler failed"}`))
			return
		}
		value = conversion.ToGo(colResult)
	}

	// The data-binding transport: the client renders. Block documents come
	// back normalized; anything else (dynamic option fetches, ad-hoc data)
	// passes through as raw JSON.
	if r.URL.Query().Get("_json") == "1" || r.URL.Query().Get("_col") != "" {
		writePluginJSON(w, value, user, plugin, r)
		return
	}

	// A dict with "blocks" is a block document (dashboards); anything else
	// falls back to the key-value view.
	if dict, ok := value.(map[string]any); ok {
		if _, hasRows := dict["rows"].([]any); hasRows {
			document := normalizePageDocument(value, user, plugin)

			tmpl, err := newTemplate("page-plugin.tmpl")
			if err != nil {
				log.Error(err.Error())
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			docJSON, err := json.Marshal(document)
			if err != nil {
				log.Error("plugin page: document", "plugin", plugin.Name, "error", err)
				renderPluginPageError(w, r, plugin, page, "failed to serialize the block document")
				return
			}
			_, data := getCommonTemplateData(r)
			data["pluginPageTitle"] = title
			data["pluginPageName"] = plugin.Name
			data["pluginPageURL"] = "/plugins/" + plugin.Name + page.Path
			data["pluginDocumentJSON"] = template.JS(docJSON)
			if err := tmpl.Execute(w, data); err != nil {
				log.Error(err.Error())
			}
			return
		}
	}

	tmpl, err := newTemplate("page-plugin.tmpl")
	if err != nil {
		log.Error(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_, data := getCommonTemplateData(r)
	data["pluginPageTitle"] = title
	data["pluginPageName"] = plugin.Name
	data["pluginPageFields"] = pluginPageFields(value)
	if err := tmpl.Execute(w, data); err != nil {
		log.Error(err.Error())
	}
}

// writePluginJSON sends a handler result over the data-binding transport.
// Block documents are normalized (validation happens server-side); any other
// value is passed through untouched so handlers can serve arbitrary JSON to
// dynamic option fetches (_data) and the Knot.plugin bridge.
func writePluginJSON(w http.ResponseWriter, value any, user *model.User, plugin *plugins.Plugin, r *http.Request) {
	// Column payloads and success-dialog envelopes may carry markdown;
	// render it server-side so the client only ever places trusted HTML.
	if dict, ok := value.(map[string]any); ok {
		if md, _ := dict["markdown"].(string); md != "" {
			var buf bytes.Buffer
			if err := mdRenderer.Convert([]byte(md), &buf); err == nil {
				dict["html"] = buf.String()
			}
			delete(dict, "markdown")
		}
		if dialog, ok := dict["dialog"].(map[string]any); ok {
			if md, _ := dialog["markdown"].(string); md != "" {
				var buf bytes.Buffer
				if err := mdRenderer.Convert([]byte(md), &buf); err == nil {
					dialog["html"] = buf.String()
				}
				delete(dialog, "markdown")
			}
		}
	}
	// Only a page layout (no _col) is normalized here: a column payload
	// may legitimately carry its own "rows" (a table's data) and must pass
	// through untouched.
	if dict, ok := value.(map[string]any); ok {
		if _, hasRows := dict["rows"].([]any); hasRows && r.URL.Query().Get("_col") == "" {
			value = normalizePageDocument(value, user, plugin)
		}
	}
	body, err := json.Marshal(value)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(body)
}

// pluginParams collects the handler's params dict: every query parameter
// except knot's control keys, overlaid by a POST body when present
// (form-encoded or JSON object; body wins on conflicts).
func pluginParams(r *http.Request) (map[string]any, error) {
	params := map[string]any{}
	for key, values := range r.URL.Query() {
		if pluginControlParams[key] || len(values) == 0 {
			continue
		}
		params[key] = values[len(values)-1]
	}

	if r.Body == nil || r.ContentLength == 0 {
		return params, nil
	}
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "application/json") {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			return nil, err
		}
		if len(body) == 0 {
			return params, nil
		}
		var decoded map[string]any
		if err := json.Unmarshal(body, &decoded); err != nil {
			return nil, err
		}
		for key, value := range decoded {
			params[key] = value
		}
		return params, nil
	}

	if err := r.ParseForm(); err != nil {
		return nil, err
	}
	for key, values := range r.PostForm {
		if len(values) > 0 {
			params[key] = values[len(values)-1]
		}
	}
	return params, nil
}

// pluginPageField is one rendered row of a handler result.
type pluginPageField struct {
	Key   string
	Value string
	IsPre bool // render the value in a <pre> block (composite values)
}

// pluginPageFields flattens a handler result into display rows: scalars
// render inline, composites (lists, dicts, non-string scalars beyond the
// primitives) render as pretty JSON. Anything html/template escapes stays
// escaped — handlers never produce markup.
func pluginPageFields(value any) []pluginPageField {
	dict, ok := value.(map[string]any)
	if !ok {
		if value == nil {
			return nil
		}
		return []pluginPageField{{Key: "result", Value: pluginPageJSON(value), IsPre: true}}
	}
	fields := make([]pluginPageField, 0, len(dict))
	for key, v := range dict {
		switch v.(type) {
		case string:
			fields = append(fields, pluginPageField{Key: key, Value: v.(string)})
		case bool, int, int64, uint32, uint64, float64:
			fields = append(fields, pluginPageField{Key: key, Value: fmt.Sprintf("%v", v)})
		case nil:
			fields = append(fields, pluginPageField{Key: key, Value: "—"})
		default:
			fields = append(fields, pluginPageField{Key: key, Value: pluginPageJSON(v), IsPre: true})
		}
	}
	return fields
}

func pluginPageJSON(v any) string {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(out)
}

// renderPluginPageError shows the failure in the page chrome rather than a
// bare status: a broken handler is a plugin bug, not a server one.
func renderPluginPageError(w http.ResponseWriter, r *http.Request, plugin *plugins.Plugin, page *plugins.Page, message string) {
	tmpl, err := newTemplate("page-plugin.tmpl")
	if err != nil {
		log.Error(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusInternalServerError)
	_, data := getCommonTemplateData(r)
	title := page.Label
	if title == "" {
		title = plugin.Name + page.Path
	}
	data["pluginPageTitle"] = title
	data["pluginPageName"] = plugin.Name
	data["pluginPageError"] = message
	if err := tmpl.Execute(w, data); err != nil {
		log.Error(err.Error())
	}
}
