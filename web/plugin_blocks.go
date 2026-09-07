package web

import (
	"fmt"
	"sync"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/plugins"
)

// Plugin pages are layout documents: the page handler returns rows of
// columns, each column declaring its type, data handler and refresh. The
// client renders the shell (with loaders) and fetches each column's data
// from its handler URL - the page path plus /<handler>. Rows and columns
// carry optional permission/group gates which knot enforces against the
// requesting user; a row left with no columns is never sent.
//
// The trust boundary: the layout skeleton is normalized here (shape, width
// and refresh clamps, gates); the column payloads a handler returns are
// trusted plugin-authored data - like the html and markdown blocks - and
// are rendered by the client, which treats them as data (textContent for
// text, colour-shaped values only for styles). Handlers may also return
// arbitrary JSON for pluginFetch callers, which is passed through as-is.

// normalizePageDocument validates the handler's layout and enforces the
// row/column gates against the requesting user. Malformed pieces are
// dropped (or the whole document becomes an error payload); gates drop
// rows/columns the user fails, and rows with no surviving columns are
// never sent.
func normalizePageDocument(value any, user *model.User, plugin *plugins.Plugin) map[string]any {
	dict, ok := value.(map[string]any)
	if !ok {
		return map[string]any{"rows": []any{}, "error": "the page handler must return a table with rows"}
	}
	rawRows, ok := dict["rows"].([]any)
	if !ok {
		return map[string]any{"rows": []any{}, "error": "the page handler must return {rows: [...]}"}
	}
	rows := make([]any, 0, len(rawRows))
	for i, rawRow := range rawRows {
		row, ok := rawRow.(map[string]any)
		if !ok {
			continue
		}
		norm := normalizeRow(i, row, user, plugin)
		if norm != nil {
			rows = append(rows, norm)
		}
	}
	return map[string]any{"rows": rows}
}

func gatePasses(table map[string]any, user *model.User, plugin *plugins.Plugin) bool {
	if permission, _ := table["permission"].(string); permission != "" {
		if !user.HasPluginPermission(plugins.QualifiedPermission(plugin.Name, permission)) {
			return false
		}
	}
	if group, _ := table["group"].(string); group != "" {
		groups := []string{group}
		if !user.HasAnyGroup(&groups) {
			return false
		}
	}
	return true
}

// layoutHandlerSet collects the handlers a normalized layout offers to the
// user it was normalized for: each column's data handler, plus each action's
// popup handler within a column. Because normalization has already applied
// the row and column gates, membership means the user can see (and therefore
// fetch) that handler's data.
func layoutHandlerSet(doc map[string]any) map[string]bool {
	set := map[string]bool{}
	rows, ok := doc["rows"].([]any)
	if !ok {
		return set
	}
	var walk func(v any)
	walk = func(v any) {
		if h, ok := v.(string); ok && h != "" {
			set[h] = true
		}
	}
	for _, rawRow := range rows {
		row, ok := rawRow.(map[string]any)
		if !ok {
			continue
		}
		columns, ok := row["columns"].([]any)
		if !ok {
			continue
		}
		for _, rawColumn := range columns {
			column, ok := rawColumn.(map[string]any)
			if !ok {
				continue
			}
			walk(column["handler"])
			actions, ok := column["actions"].([]any)
			if !ok {
				continue
			}
			for _, rawAction := range actions {
				action, ok := rawAction.(map[string]any)
				if !ok {
					continue
				}
				walk(action["handler"])
			}
		}
	}
	return set
}

// layoutPresenceCache memoizes, per plugin/page/user/query, the handler set
// a page's normalized layout offers — the answer to "may this user fetch
// this undeclared handler". Without it every column fetch would re-run the
// page's layout handler, usually the heaviest function in the plugin, and a
// page's columns fetch as a burst. Entries live for layoutPresenceTTL: a
// handler the layout withdraws (or a gate revoked via a role edit) stays
// fetchable for at most that window — the same staleness the role cache
// tolerates. GET only: a POST body can change what the layout offers.
type layoutPresenceCache struct {
	mu      sync.Mutex
	entries map[string]layoutPresenceEntry
	ttl     time.Duration
	hits    int
	misses  int
}

type layoutPresenceEntry struct {
	handlers map[string]bool
	expires  time.Time
}

const (
	layoutPresenceTTL         = 5 * time.Second
	layoutPresenceMaxEntries  = 8192
)

var layoutPresence = &layoutPresenceCache{entries: map[string]layoutPresenceEntry{}, ttl: layoutPresenceTTL}

// get returns the memoized handler set for a key, if fresh.
func (c *layoutPresenceCache) get(key string) (map[string]bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		c.misses++
		return nil, false
	}
	if time.Now().After(entry.expires) {
		delete(c.entries, key)
		c.misses++
		return nil, false
	}
	c.hits++
	return entry.handlers, true
}

// set memoizes a handler set. On overflow the cache clears wholesale: an
// entry costs one layout evaluation to rebuild, and the working set of live
// users x pages x queries is far below the cap in practice.
func (c *layoutPresenceCache) set(key string, handlers map[string]bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= layoutPresenceMaxEntries {
		c.entries = map[string]layoutPresenceEntry{}
	}
	c.entries[key] = layoutPresenceEntry{handlers: handlers, expires: time.Now().Add(c.ttl)}
}

func (c *layoutPresenceCache) stats() (hits, misses int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hits, c.misses
}

func (c *layoutPresenceCache) resetForTest(ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = map[string]layoutPresenceEntry{}
	c.ttl = ttl
	c.hits, c.misses = 0, 0
}

func normalizeRow(index int, row map[string]any, user *model.User, plugin *plugins.Plugin) map[string]any {
	if !gatePasses(row, user, plugin) {
		return nil
	}
	out := map[string]any{}
	if title, _ := row["title"].(string); title != "" {
		out["title"] = title
	}
	if style, _ := row["style"].(string); style == "card" {
		out["style"] = "card"
	}
	rawColumns, ok := row["columns"].([]any)
	if !ok || len(rawColumns) == 0 {
		return nil
	}
	columns := make([]any, 0, len(rawColumns))
	for j, rawColumn := range rawColumns {
		column, ok := rawColumn.(map[string]any)
		if !ok {
			continue
		}
		if !gatePasses(column, user, plugin) {
			continue
		}
		norm := map[string]any{}
		for _, key := range []string{"id", "type", "title", "handler", "actions", "options"} {
			if v, ok := column[key]; ok {
				norm[key] = v
			}
		}
		if id, _ := column["id"].(string); id == "" {
			norm["id"] = fmt.Sprintf("r%dc%d", index, j)
		}
		// Stats are KPI cells: they default to one grid track so a row of
		// them reads as a KPI row; everything else defaults to full width.
		defWidth := 4
		if t, _ := column["type"].(string); t == "stat" {
			defWidth = 1
		}
		norm["width"] = clampInt(intOf(column["width"], defWidth), 1, 4)
		if column["refresh"] != nil {
			norm["refresh"] = clampInt(intOf(column["refresh"], 0), 5, 3600)
		}
		columns = append(columns, norm)
	}
	if len(columns) == 0 {
		return nil
	}
	out["columns"] = columns
	return out
}

// mdRenderer renders markdown blocks. Like the html block, markdown is
// trusted plugin-authored content (plugins are admin-installed) and may
// embed raw HTML; the client styles it with knot's pb-markdown prose.
var mdRenderer = goldmark.New(goldmark.WithExtensions(extension.GFM))

func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func intOf(v any, def int) int {
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case uint64:
		return int(t)
	case float64:
		return int(t)
	}
	return def
}
