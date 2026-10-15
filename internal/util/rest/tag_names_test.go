package rest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// agentProtocolDirs carry the agent and cluster wire formats and the file
// store's records, which never travel over REST and are free to name
// fields for themselves.
var agentProtocolDirs = []string{
	"internal/agentapi/msg",
	"internal/agentlink",
	"internal/cluster/leafmsg",
	"internal/filestore",
}

// TestEveryJSONFieldHasMatchingMsgPackTag keeps msgpack replies and
// requests equal to JSON ones: msgpack names a field by its msgpack tag, or
// else its Go name, so every field JSON names must carry a msgpack tag
// naming it the same, "-" included. A field JSON names by its Go name needs
// none.
func TestEveryJSONFieldHasMatchingMsgPackTag(t *testing.T) {
	root, _ := filepath.Abs("../../..")
	var problems []string
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		if d.IsDir() {
			if d.Name() == "node_modules" || d.Name() == ".git" {
				return filepath.SkipDir
			}
			for _, skip := range agentProtocolDirs {
				if rel == filepath.FromSlash(skip) {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), p, nil, 0)
		if err != nil {
			return nil
		}
		ast.Inspect(file, func(n ast.Node) bool {
			field, ok := n.(*ast.Field)
			if !ok || field.Tag == nil {
				return true
			}
			raw, err := strconv.Unquote(field.Tag.Value)
			if err != nil {
				return true
			}
			tag := reflect.StructTag(raw)
			jsonTag, hasJSON := tag.Lookup("json")
			jsonName, _, _ := strings.Cut(jsonTag, ",")
			if !hasJSON || jsonName == "" {
				return true
			}
			msgpackTag, hasMsgPack := tag.Lookup("msgpack")
			msgpackName, _, _ := strings.Cut(msgpackTag, ",")
			if !hasMsgPack || msgpackName != jsonName {
				problems = append(problems, rel+": "+raw)
			}
			return true
		})
		return nil
	})
	for _, p := range problems {
		t.Errorf("needs a msgpack tag naming the field as its json tag does: %s", p)
	}
}
