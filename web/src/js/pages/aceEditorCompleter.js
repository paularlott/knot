/**
 * ACE Editor Custom Completer
 *
 * Provides intelligent autocompletion for ACE editor instances with support for:
 * - Module imports (e.g., "import knot.mcp")
 * - Module functions (e.g., "mcp.get_string()")
 * - Class instance methods based on type inference from factory function calls
 *
 * Usage:
 *   AceEditorCompleter.setup(editor, completions, { debug: false });
 */
(function () {
  "use strict";

  // Track if we've already added the completer to avoid duplicates
  let completerRegistered = false;
  let registeredCompletions = [];
  let factoryFunctions = {};
  let debugMode = false;

  /**
   * Setup custom completions for an ACE editor.
   * Can be called multiple times - completions are merged, completer registered once.
   */
  function setup(editor, completions, options) {
    options = options || {};
    debugMode = options.debug || false;

    if (!completions || completions.length === 0) {
      return;
    }

    // Merge new completions with existing ones (avoid duplicates by module name)
    completions.forEach(function (lib) {
      const existing = registeredCompletions.find(function (l) {
        return l.module === lib.module;
      });
      if (!existing) {
        registeredCompletions.push(lib);
      }
    });

    // Rebuild factory functions map
    factoryFunctions = buildFactoryFunctionMap(registeredCompletions);

    if (debugMode) {
      console.log("ACE Completer: Factory functions map:", factoryFunctions);
      console.log(
        "ACE Completer: Registered completions:",
        registeredCompletions.length
      );
    }

    // Register the completer only once
    if (!completerRegistered) {
      registerCompleter();
      completerRegistered = true;
    }
  }

  /**
   * Builds a map of factory function patterns to their return types.
   * Parses the "returns" field in format "TypeName - Description"
   */
  function buildFactoryFunctionMap(completions) {
    const map = {};

    completions.forEach(function (lib) {
      if (lib.functions) {
        lib.functions.forEach(function (func) {
          if (func.returns && func.returns.includes(" - ")) {
            const returnType = func.returns.split(" - ")[0].trim();
            const parts = lib.module.split(".");
            const alias = parts[parts.length - 1];
            map[alias + "." + func.name] = returnType;
            map[lib.module + "." + func.name] = returnType;
          }
        });
      }
    });

    return map;
  }

  /**
   * Finds the type of a variable by scanning the document for its assignment.
   * Looks for patterns like: client = sl.ai.new_client(...)
   */
  function findVariableType(session, varName, currentRow) {
    if (debugMode) {
      console.log(
        "ACE Completer: Looking for variable type:",
        varName,
        "from row",
        currentRow
      );
    }

    for (let row = currentRow; row >= 0; row--) {
      const lineText = session.getLine(row);
      const assignmentPattern = new RegExp(
        "^\\s*" +
          varName +
          "\\s*=\\s*([a-z_][a-z0-9_]*(?:\\.[a-z_][a-z0-9_]*)*)\\s*\\(",
        "i"
      );
      const match = lineText.match(assignmentPattern);

      if (match) {
        const funcCall = match[1];
        if (debugMode) {
          console.log(
            "ACE Completer: Found assignment:",
            varName,
            "=",
            funcCall,
            "on row",
            row
          );
        }

        for (const pattern in factoryFunctions) {
          if (funcCall.toLowerCase() === pattern.toLowerCase()) {
            if (debugMode) {
              console.log(
                "ACE Completer: Matched factory function:",
                pattern,
                "-> type:",
                factoryFunctions[pattern]
              );
            }
            return factoryFunctions[pattern];
          }
        }
      }
    }

    if (debugMode) {
      console.log("ACE Completer: No type found for variable:", varName);
    }
    return null;
  }

  /**
   * Builds HTML documentation for completion popup.
   */
  function buildDocHTML(name, signature, description, returns) {
    let html = "<b>" + name + "</b>";
    if (signature) {
      html += "<br/><code>" + signature + "</code>";
    }
    if (description) {
      html += "<br/>" + description;
    }
    if (returns) {
      html += "<br/><i>Returns:</i> " + returns;
    }
    return html;
  }

  /**
   * Registers the custom completer with ACE's language tools.
   */
  function registerCompleter() {
    const customCompleter = {
      getCompletions: function (editor, session, pos, prefix, callback) {
        const line = session.getLine(pos.row).substring(0, pos.column);

        if (debugMode) {
          console.log(
            "ACE Completer: getCompletions - line:",
            line,
            "prefix:",
            prefix,
            "pos:",
            pos
          );
        }

        const completionResults = [];
        const dotMatch = line.match(/([a-z_][a-z0-9_]*)\.\s*([a-z_]*)$/i);

        registeredCompletions.forEach(function (lib) {
          // Import completions (import sl. or from sl.)
          if (
            line.match(/^\s*import\s+[a-z_.]*$/i) ||
            line.match(/^\s*from\s+[a-z_.]*$/i)
          ) {
            const importMatch = line.match(
              /^\s*(?:import|from)\s+([a-z_.]*)$/i
            );
            const typed = importMatch ? importMatch[1].toLowerCase() : "";
            if (lib.module.toLowerCase().startsWith(typed)) {
              completionResults.push({
                caption: lib.module,
                value: lib.module,
                score: 1000,
                meta: "module",
                docHTML: "<b>" + lib.module + "</b><br/>" + lib.description,
              });
            }
          }

          // Module function completions (e.g., ai.get_models())
          if (dotMatch) {
            const objectName = dotMatch[1].toLowerCase();
            const methodPrefix = (dotMatch[2] || "").toLowerCase();
            const parts = lib.module.split(".");
            const moduleAlias = parts[parts.length - 1].toLowerCase();

            if (
              moduleAlias === objectName ||
              lib.module.toLowerCase() === objectName
            ) {
              if (lib.functions) {
                lib.functions.forEach(function (func) {
                  if (func.name.toLowerCase().startsWith(methodPrefix)) {
                    completionResults.push({
                      caption: func.name,
                      value: func.signature || func.name + "()",
                      score: 900,
                      meta: lib.module,
                      docHTML: buildDocHTML(
                        func.name,
                        func.signature,
                        func.description,
                        func.returns
                      ),
                    });
                  }
                });
              }
            }
          }

          // Class method completions based on variable type inference
          if (lib.classes && dotMatch) {
            const varName = dotMatch[1];
            const methodPrefix = (dotMatch[2] || "").toLowerCase();
            const varType = findVariableType(session, varName, pos.row);

            if (varType) {
              lib.classes.forEach(function (cls) {
                const typeMatches =
                  varType.toLowerCase() === cls.name.toLowerCase() ||
                  varType.toLowerCase().includes(cls.name.toLowerCase());

                if (typeMatches) {
                  if (debugMode) {
                    console.log(
                      "ACE Completer: Type matches class:",
                      cls.name,
                      "adding methods"
                    );
                  }
                  cls.methods.forEach(function (method) {
                    if (method.name.toLowerCase().startsWith(methodPrefix)) {
                      completionResults.push({
                        caption: method.name,
                        value: method.signature || method.name + "()",
                        score: 950,
                        meta: cls.name,
                        docHTML: buildDocHTML(
                          method.name,
                          method.signature,
                          method.description,
                          method.returns
                        ),
                      });
                    }
                  });
                }
              });
            }
          }
        });

        if (debugMode) {
          console.log(
            "ACE Completer: Returning completions:",
            completionResults.length
          );
        }

        callback(null, completionResults);
      },
    };

    const langTools = ace.require("ace/ext/language_tools");
    langTools.addCompleter(customCompleter);

    if (debugMode) {
      console.log("ACE Completer: Registered custom completer");
    }
  }

  // Export to window
  window.AceEditorCompleter = {
    setup: setup,
  };
})();
