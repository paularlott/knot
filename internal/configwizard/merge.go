package configwizard

import (
	"strings"
)

// Merging: the wizard generates the full set of tables it manages, but a
// hand-edited config can contain anything — comments, keys and whole
// sections the wizard knows nothing about. On overwrite, the wizard's
// values win for the keys it manages, and everything else is preserved:
//
//   - tables the wizard does not manage are kept verbatim (comments too)
//   - unknown keys inside managed tables are kept (appended after the
//     wizard's keys)
//   - comments inside managed tables are not preserved (those tables are
//     regenerated); comments everywhere else are
//
// mergeConfig rewrites existing with generated applied on top. If existing
// is empty or unparsable into sections, generated is returned unchanged.

type tomlBlock struct {
	name   string // "" for the preamble before the first table header
	header string // the header line as written, e.g. "[server.badgerdb]"
	lines  []string
}

var tableHeaderChars = func(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
		r == '_' || r == '-' || r == '.' || r == '"' || r == '\''
}

// splitTables breaks a TOML document into an ordered preamble + table
// blocks. A line is a table header only when it is exactly `[name]`
// (optionally followed by a comment) and the name looks like a table path —
// this keeps multi-line array continuation lines such as `["A|x", "B|y"]`
// from being mistaken for headers.
func splitTables(doc string) []tomlBlock {
	var blocks []tomlBlock
	current := tomlBlock{}

	for _, line := range strings.Split(doc, "\n") {
		trimmed := strings.TrimSpace(line)
		if name, ok := tableHeader(trimmed); ok {
			if current.name != "" || len(current.lines) > 0 {
				blocks = append(blocks, current)
			}
			current = tomlBlock{name: name, header: line, lines: nil}
			continue
		}
		current.lines = append(current.lines, line)
	}
	if current.name != "" || len(current.lines) > 0 {
		blocks = append(blocks, current)
	}
	return blocks
}

func tableHeader(line string) (string, bool) {
	if !strings.HasPrefix(line, "[") {
		return "", false
	}
	// Allow a trailing comment after the closing bracket.
	if idx := strings.Index(line, "#"); idx > 0 {
		line = strings.TrimSpace(line[:idx])
	}
	if !strings.HasSuffix(line, "]") {
		return "", false
	}
	name := line[1 : len(line)-1]
	if name == "" || strings.ContainsAny(name, "=[]{}|#,") {
		return "", false
	}
	if strings.TrimFunc(name, tableHeaderChars) != "" {
		return "", false
	}
	return name, true
}

// keyOf returns the key name when the line assigns a top-level table key
// (key = value), or "" for comments, blanks and continuation lines.
func keyOf(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return ""
	}
	eq := strings.Index(trimmed, "=")
	if eq <= 0 {
		return ""
	}
	key := strings.TrimSpace(trimmed[:eq])
	if strings.ContainsAny(key, "[]. \"'") {
		return ""
	}
	return key
}

func mergeConfig(existing, generated string) string {
	if strings.TrimSpace(existing) == "" {
		return generated
	}

	existingBlocks := splitTables(existing)
	generatedBlocks := splitTables(generated)

	generatedByName := map[string]*tomlBlock{}
	for i := range generatedBlocks {
		generatedByName[generatedBlocks[i].name] = &generatedBlocks[i]
	}

	var out []string
	used := map[string]bool{}

	// Walk the existing document in order, replacing managed tables with
	// the generated version (plus any unknown keys the user had added
	// inside them) and keeping everything else verbatim.
	for _, block := range existingBlocks {
		gen, managed := generatedByName[block.name]
		if !managed {
			out = append(out, blockLines(block)...)
			continue
		}
		used[block.name] = true
		out = append(out, blockLines(mergeBlock(block, *gen))...)
	}

	// Append generated tables that the existing file didn't have.
	for _, gen := range generatedBlocks {
		if !used[gen.name] {
			out = append(out, blockLines(gen)...)
		}
	}

	return strings.Join(out, "\n")
}

// mergeBlock applies the generated table body over the existing one,
// appending existing keys the generated version doesn't define so
// hand-added keys inside managed tables survive. Unknown keys keep their
// continuation lines (multi-line arrays), blank lines and comments between
// them are carried along.
func mergeBlock(existing, generated tomlBlock) tomlBlock {
	genKeys := map[string]bool{}
	for _, line := range generated.lines {
		if k := keyOf(line); k != "" {
			genKeys[k] = true
		}
	}

	type entry struct {
		key   string
		lines []string
	}
	var entries []entry
	for _, line := range existing.lines {
		k := keyOf(line)
		if k != "" || len(entries) == 0 {
			entries = append(entries, entry{key: k, lines: []string{line}})
			continue
		}
		entries[len(entries)-1].lines = append(entries[len(entries)-1].lines, line)
	}

	merged := tomlBlock{name: generated.name, header: generated.header, lines: generated.lines}
	for _, e := range entries {
		if e.key != "" && !genKeys[e.key] {
			merged.lines = append(merged.lines, e.lines...)
		}
	}
	return merged
}

func blockLines(block tomlBlock) []string {
	if block.name == "" {
		return block.lines
	}
	return append([]string{block.header}, block.lines...)
}
