package web

import (
	"fmt"
	"regexp"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/plugins"
)

// Plugin pages are layout documents: the page handler returns rows of
// columns, each column declaring its type, data handler and refresh. The
// client renders the shell (with loaders) and fetches each column's data
// from the same URL with ?_col=<id>. Rows and columns carry optional
// permission/group gates which knot enforces against the requesting user;
// a row left with no columns is never sent.

var pluginControlParams = map[string]bool{
	"_json": true,
}

// maxPluginPopupDepth caps popup-in-popup nesting: the document renders at
// depth 0, so three levels of popups are available before the error block.
const maxPluginPopupDepth = 3

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

func normalizeTable(block map[string]any, table map[string]any) error {
	rawColumns, ok := table["columns"].([]any)
	if !ok || len(rawColumns) == 0 {
		return fmt.Errorf("requires columns")
	}
	columns := make([]any, 0, len(rawColumns))
	keys := make([]string, 0, len(rawColumns))
	for _, raw := range rawColumns {
		col, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("columns entries must be tables with key and label")
		}
		key, _ := col["key"].(string)
		label, _ := col["label"].(string)
		if key == "" {
			return fmt.Errorf("columns entries require key")
		}
		if label == "" {
			label = key
		}
		out := map[string]any{"key": key, "label": label}
		if badge, _ := col["badge"].(bool); badge {
			out["badge"] = true
		}
		columns = append(columns, out)
		keys = append(keys, key)
	}
	block["columns"] = columns

	rows := make([]any, 0)
	if rawRows, ok := table["rows"].([]any); ok {
		for _, raw := range rawRows {
			row, ok := raw.(map[string]any)
			if !ok {
				return fmt.Errorf("rows entries must be tables")
			}
			cells := map[string]any{}
			for _, key := range keys {
				cells[key] = scalarString(row[key])
			}
			rows = append(rows, cells)
		}
	}
	block["rows"] = rows
	return nil
}

// hexColorRe constrains accent/bar colours: they are interpolated into
// styles client-side, so only #rgb/#rrggbb(/#rrggbbaa) shapes are allowed.
var hexColorRe = regexp.MustCompile(`^#[0-9a-fA-F]{3,8}$`)

var regexpEmptyOnly = regexp.MustCompile(`^\s*$`)

// mdRenderer renders markdown blocks. Like the html block, markdown is
// trusted plugin-authored content (plugins are admin-installed) and may
// embed raw HTML; the client styles it with knot's pb-markdown prose.
var mdRenderer = goldmark.New(goldmark.WithExtensions(extension.GFM))

const defaultBarColor = "#3b82f6"

// floatOf coerces a scriptling number to float64.
func floatOf(v any) (float64, error) {
	switch t := v.(type) {
	case int:
		return float64(t), nil
	case int64:
		return float64(t), nil
	case uint32:
		return float64(t), nil
	case uint64:
		return float64(t), nil
	case float64:
		return t, nil
	default:
		return 0, fmt.Errorf("expected a number, got %T", v)
	}
}

func normalizeChart(block map[string]any, table map[string]any) error {
	chartType, _ := table["chart_type"].(string)
	if chartType == "" {
		if t, _ := table["chart"].(string); t != "" {
			chartType = t
		}
	}
	switch chartType {
	case "line", "bar", "doughnut", "pie":
	default:
		return fmt.Errorf("chart_type must be one of line, bar, doughnut, pie")
	}
	block["chart_type"] = chartType

	labels, err := stringList(table["labels"])
	if err != nil || len(labels) == 0 {
		return fmt.Errorf("requires labels")
	}
	block["labels"] = labels

	rawDatasets, ok := table["datasets"].([]any)
	if !ok || len(rawDatasets) == 0 {
		return fmt.Errorf("requires datasets")
	}
	datasets := make([]any, 0, len(rawDatasets))
	for i, raw := range rawDatasets {
		ds, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("datasets[%d] must be a table", i)
		}
		name, _ := ds["name"].(string)
		if name == "" {
			return fmt.Errorf("datasets[%d] requires name", i)
		}
		rawData, ok := ds["data"].([]any)
		if !ok {
			return fmt.Errorf("datasets[%d] requires data", i)
		}
		data := make([]float64, 0, len(rawData))
		for _, point := range rawData {
			value, err := floatOf(point)
			if err != nil {
				return fmt.Errorf("datasets[%d].data must be numbers", i)
			}
			data = append(data, value)
		}
		if len(data) != len(labels) {
			return fmt.Errorf("datasets[%d] has %d points but there are %d labels", i, len(data), len(labels))
		}
		out := map[string]any{"name": name, "data": data}
		if color, _ := ds["color"].(string); color != "" {
			out["color"] = color
		}
		datasets = append(datasets, out)
	}
	block["datasets"] = datasets

	height := 240
	switch t := table["height"].(type) {
	case int:
		height = t
	case int64:
		height = int(t)
	case float64:
		height = int(t)
	}
	block["height"] = clampInt(height, 120, 600)
	return nil
}

func normalizeFormFields(v any) ([]any, error) {
	raw, ok := v.([]any)
	if !ok || len(raw) == 0 {
		return nil, fmt.Errorf("requires fields")
	}
	fields := make([]any, 0, len(raw))
	for i, item := range raw {
		f, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("fields[%d] must be a table", i)
		}
		name, _ := f["name"].(string)
		if name == "" {
			return nil, fmt.Errorf("fields[%d] requires name", i)
		}
		field := map[string]any{"name": name}
		label, _ := f["label"].(string)
		if label == "" {
			label = name
		}
		field["label"] = label
		fieldType, _ := f["type"].(string)
		if fieldType == "" {
			fieldType = "text"
		}
		switch fieldType {
		case "text", "number", "select", "autocomplete", "hidden":
		default:
			return nil, fmt.Errorf("fields[%d]: type must be text, number, select, autocomplete or hidden", i)
		}
		field["type"] = fieldType
		if fieldType == "select" {
			options, err := stringList(f["options"])
			if err != nil || len(options) == 0 {
				return nil, fmt.Errorf("fields[%d]: select requires options", i)
			}
			field["options"] = options
		}
		if fieldType == "autocomplete" {
			// Suggestions may be fixed (options) or fetched from the plugin
			// when the form renders (dynamic_options: the client asks this
			// page's handler with _data=<name> and reads {"options": [...]}).
			// Options are key -> text: the user picks by text, the form
			// stores the key (a plain string means key == text). Typing a
			// value outside the list is always allowed.
			if options, err := stringList(f["options"]); err == nil && len(options) > 0 {
				field["options"] = options
			}
			if dynamic, _ := f["dynamic_options"].(bool); dynamic {
				field["dynamic_options"] = true
			}
		}
		field["value"] = scalarString(f["value"])
		if placeholder, _ := f["placeholder"].(string); placeholder != "" {
			field["placeholder"] = placeholder
		}
		fields = append(fields, field)
	}
	return fields, nil
}

func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func scalarString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	default:
		return fmt.Sprintf("%v", t)
	}
}

func stringList(v any) ([]string, error) {
	list, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("expected a list")
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		out = append(out, scalarString(item))
	}
	return out, nil
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
