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

// pluginLogoURLs returns the plugin-provided site-logo URLs (light, dark)
// for the template data, or ("", "") when no plugin claims the site logo or
// an explicit server.ui.logo_url config is set — admin config beats plugins,
// which beat the built-in knot logo. One resolution pass serves both.
func pluginLogoURLs(cfg *config.ServerConfig) (string, string) {
	if cfg.UI.LogoURL != "" {
		return "", ""
	}
	if registry := plugins.GetRegistry(); registry != nil {
		light, dark := registry.SiteLogoURLs()
		return light, dark
	}
	return "", ""
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

// resolvePluginDispatch maps a request path to a page render or a handler
// call. Exact declared pages win; otherwise the longest declared page whose
// path is a prefix leaves a single handler segment (/plugins/x/page/handler),
// and a lone segment at the plugin root (/plugins/x/handler) serves only
// handlers with a [[tool.knot.handlers]] declaration — the declaration is
// the opt-in that a handler is addressable without a page, and its
// permission is the handler's gate. Undeclared handlers inherit the
// calling page's gate and are reachable only through a page path. A nil
// plugin means 404; a nil page (root dispatch) is always paired with a
// declared handler.
func resolvePluginDispatch(registry *plugins.Registry, pluginName, requestPath string) (*plugins.Plugin, *plugins.Page, string) {
	if plugin, page := registry.Page(pluginName, requestPath); plugin != nil {
		return plugin, page, ""
	}
	plugin := registry.ByName(pluginName)
	if plugin == nil || requestPath == "/" {
		return nil, nil, ""
	}

	var best *plugins.Page
	bestLen := -1
	for i := range plugin.Pages {
		page := &plugin.Pages[i]
		if !strings.HasPrefix(requestPath, page.Path+"/") {
			continue
		}
		handler := requestPath[len(page.Path)+1:]
		if strings.Contains(handler, "/") || !plugins.ValidHandlerName(handler) {
			continue
		}
		if len(page.Path) > bestLen {
			best, bestLen = page, len(page.Path)
		}
	}
	if best != nil {
		return plugin, best, requestPath[bestLen+1:]
	}

	// Plugin-root handler: only declared handlers are addressable here.
	handler := strings.TrimPrefix(requestPath, "/")
	if plugin.HandlerDecl(handler) != nil {
		return plugin, nil, handler
	}
	return nil, nil, ""
}

// HandlePluginPage dispatches a declared plugin page: knot checks the gate,
// then evaluates the plugin's entry file in a fresh run-as-user environment
// and calls the declared handler, rendering its returned value. The
// environment (and everything in it) is discarded with the request — plugin
// code runs only here, never at boot.
//
// Handler URLs: /plugins/<name>/<page-path>/<handler> calls that handler,
// and /plugins/<name>/<handler> calls a [[tool.knot.handlers]]-declared
// handler at the plugin root — the addressable ajax surface any page (or
// any other plugin's page) fetches. The response is always JSON. The gate
// is the calling page's permission unless the handler has its own
// declaration, which is authoritative everywhere the handler is called —
// the same semantics as row/column gates.
func HandlePluginPage(w http.ResponseWriter, r *http.Request) {
	pluginName := r.PathValue("plugin_name")
	requestPath := "/" + strings.Trim(r.PathValue("path"), "/")

	registry := plugins.GetRegistry()
	if registry == nil {
		http.NotFound(w, r)
		return
	}
	plugin, page, handler := resolvePluginDispatch(registry, pluginName, requestPath)
	if plugin == nil {
		http.NotFound(w, r)
		return
	}

	user := r.Context().Value("user").(*model.User)

	// knot enforces the gate — a plugin cannot forget or skip it. A called
	// handler with its own declaration carries that gate instead of the
	// page's.
	gatePermission := ""
	if page != nil {
		gatePermission = page.Permission
	}
	if handler != "" {
		if decl := plugin.HandlerDecl(handler); decl != nil {
			gatePermission = decl.Permission
		}
	}
	if !user.PassesPluginGate(gatePermission) {
		showPageForbidden(w, r)
		return
	}

	client := apiclient.NewMuxClient(user)

	ctx, cancel := context.WithTimeout(r.Context(), pluginPageTimeout())
	defer cancel()

	// A pooled env for the plugin, Reset and rebound to this user per lease
	// (clean module state, their identity, the entry re-evaluated from the
	// parsed-program cache): dispatch skips the interpreter build and
	// library registration a fresh env pays on every request.
	env, err := service.AcquirePluginEnv(ctx, client, user, plugin)
	if err != nil {
		log.Error("plugin page: env", "plugin", plugin.Name, "error", err)
		renderPluginPageError(w, r, plugin, page, fmt.Sprintf("plugin environment failed: %v", err))
		return
	}
	defer service.ReleasePluginEnv(env, plugin)

	// The request reaches the handler as the params dict: query parameters,
	// plus — for actions — a POST body (form-encoded or JSON), which wins on
	// key conflicts. Every query parameter flows through, including _data,
	// which dynamic-option fetches use to name the field they want.
	params, err := pluginParams(r)
	if err != nil {
		renderPluginPageError(w, r, plugin, page, "invalid request body")
		return
	}
	if err := env.SetObjectVar("request", conversion.FromGo(service.RequestObject(r.Method, r.URL.Path))); err != nil {
		renderPluginPageError(w, r, plugin, page, "failed to set the request object")
		return
	}
	if err := env.SetObjectVar("user", service.NewUserObject(user)); err != nil {
		renderPluginPageError(w, r, plugin, page, "failed to set the user object")
		return
	}
	if err := env.SetObjectVar("params", conversion.FromGo(params)); err != nil {
		log.Error("plugin page: params", "plugin", plugin.Name, "error", err)
		renderPluginPageError(w, r, plugin, page, "failed to set handler params")
		return
	}

	// Handler-URL dispatch: call the addressed handler directly and answer
	// as JSON. A declared handler stands on its own gate; an undeclared one
	// is reachable only through a page, and the column gates must hold at
	// fetch time too — its name has to appear in the layout as the
	// requesting user sees it (row/column gates applied), as a column's
	// handler or an action's popup handler. That answer is memoized per
	// user and query for a few seconds (GET only), so a page's column burst
	// costs one layout run instead of one per column.
	if handler != "" {
		if page != nil && plugin.HandlerDecl(handler) == nil {
			cacheable := r.Method == http.MethodGet
			var key string
			var offered map[string]bool
			var cached bool
			if cacheable {
				key = plugin.Name + "\x00" + page.Path + "\x00" + user.Id + "\x00" + user.Username + "\x00" + r.URL.RawQuery
				offered, cached = layoutPresence.get(key)
			}
			if !cached {
				layoutResult, err := env.CallFunctionWithContext(ctx, page.Handler)
				if err != nil {
					log.Error("plugin page: layout for handler gate", "plugin", plugin.Name, "handler", page.Handler, "error", err)
					w.Header().Set("Content-Type", "application/json; charset=utf-8")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write([]byte(`{"error":"page layout failed"}`))
					return
				}
				offered = layoutHandlerSet(normalizePageDocument(conversion.ToGo(layoutResult), user, plugin))
				if cacheable {
					layoutPresence.set(key, offered)
				}
			}
			if !offered[handler] {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte(`{"error":"handler not available for this user"}`))
				return
			}
		}
		result, err := env.CallFunctionWithContext(ctx, handler)
		if err != nil {
			log.Error("plugin handler", "plugin", plugin.Name, "handler", handler, "error", err)
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"handler failed"}`))
			return
		}
		writePluginJSON(w, conversion.ToGo(result))
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

// writePluginJSON sends a handler result as JSON. Handler payloads pass
// through untouched — a dict's "rows" is data (a table's rows), never a
// page layout — so handlers can serve arbitrary JSON to dynamic option
// fetches (_data) and pluginFetch callers. Markdown in a payload or a
// success dialog is rendered server-side so the client only ever places
// trusted HTML.
func writePluginJSON(w http.ResponseWriter, value any) {
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
	body, err := json.Marshal(value)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(body)
}

// pluginParams collects the handler's params dict: every query parameter,
// overlaid by a POST body when present (form-encoded or JSON object; body
// wins on conflicts).
func pluginParams(r *http.Request) (map[string]any, error) {
	params := map[string]any{}
	for key, values := range r.URL.Query() {
		if len(values) == 0 {
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
// bare status: a broken handler is a plugin bug, not a server one. page may
// be nil (plugin-root handler dispatch has no page context).
func renderPluginPageError(w http.ResponseWriter, r *http.Request, plugin *plugins.Plugin, page *plugins.Page, message string) {
	tmpl, err := newTemplate("page-plugin.tmpl")
	if err != nil {
		log.Error(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusInternalServerError)
	_, data := getCommonTemplateData(r)
	title := ""
	if page != nil {
		title = page.Label
	}
	if title == "" {
		title = plugin.Name + r.URL.Path
	}
	data["pluginPageTitle"] = title
	data["pluginPageName"] = plugin.Name
	data["pluginPageError"] = message
	if err := tmpl.Execute(w, data); err != nil {
		log.Error(err.Error())
	}
}
