export function setSpecCompleter(editor, definitions) {
  const completions = Array.isArray(definitions) ? definitions : [];

  editor.completers = [
    {
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
  ];

  editor.setOptions({
    enableBasicAutocompletion: true,
    enableLiveAutocompletion: true,
    enableSnippets: false,
  });
}
