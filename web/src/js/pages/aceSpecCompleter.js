// Derive the "suffix" form of a full variable entry, e.g. "${{ .space.id }}" ->
// "space.id" and the partial "${{ .custom." -> "custom.". Used to offer the
// variable set when completing a nested path such as ${{ .stack.<key>.<...> }}.
function variableSuffix(entry) {
  const v = entry.value || "";
  return v
    .replace(/^\$\{\{\s*\.?/, "")
    .replace(/^\./, "")
    .replace(/\s*\}\}$/, "");
}

export function setSpecCompleter(editor, definitions) {
  const completions = Array.isArray(definitions) ? definitions : [];

  // Suffix forms of the variable entries (excluding the .stack. opener, which
  // is meaningless once nested). Offered when the cursor sits after a nested
  // path opener like ${{ .stack.<key>.
  const nestedSuffixes = completions
    .filter((d) => /^\$\{\{\s*\./.test(d.value || "") && !/\.stack\./.test(d.value))
    .map((d) => ({
      caption: variableSuffix(d),
      value: variableSuffix(d),
      meta: d.meta,
      score: d.score,
      docHTML: d.docHTML,
    }));

  editor.completers = [
    {
      // Standard prefix matcher over the supplied definitions (keyword snippets
      // and full variable entries).
      getCompletions(_editor, _session, _pos, prefix, callback) {
        const lowerPrefix = (prefix || "").toLowerCase();
        const matches = completions.filter((item) => {
          if (!lowerPrefix) return true;
          return (
            item.caption.toLowerCase().startsWith(lowerPrefix) ||
            item.value.toLowerCase().startsWith(lowerPrefix)
          );
        });
        callback(null, matches);
      },
    },
    {
      // Context-aware completer: when the cursor is directly after a nested
      // path opener such as ${{ .stack.<key>., offer the variable set as
      // suffixes (space.id, custom., user.username, ...) so cross-space
      // references can be completed the same way as top-level variables.
      getCompletions(_editor, session, pos, prefix, callback) {
        const line = session.getLine(pos.row) || "";
        const before = line.slice(0, pos.column);
        // Detect completion of a field after a stack-sibling reference, in
        // either form:
        //   ${{ .stack.<key>.            (identifier-safe keys)
        //   ${{ (index .stack "<key>").  (required when the key contains a "-")
        const dotted = before.match(/\$\{\{\s*\.stack\.[\w]+\.\s*([\w]*)$/);
        const indexed = before.match(/\(index\s+\.stack\s+"[\w-]+"\)\.\s*([\w]*)$/);
        const m = dotted || indexed;
        if (!m) {
          callback(null, []);
          return;
        }
        const partial = m[1].toLowerCase();
        const matches = nestedSuffixes.filter((item) => {
          if (!partial) return true;
          return item.caption.toLowerCase().startsWith(partial);
        });
        callback(null, matches);
      },
    },
  ];

  editor.setOptions({
    enableBasicAutocompletion: true,
    enableLiveAutocompletion: true,
    enableSnippets: false,
  });
}
