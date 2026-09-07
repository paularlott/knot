#!/usr/bin/env node
/**
 * Generate the ACE editor completion data for knot.* libraries from the
 * type stubs in ../knot-vscode (the IntelliSense source of truth).
 *
 * Usage:   node scripts/generate-knot-completions.mjs
 * Output:  web/src/js/pages/knotCompletions.js
 *
 * The generated file is imported by scriptCompletions.js alongside the
 * scriptling completions. Re-run whenever the knot vscode stubs change.
 *
 * Descriptions come from the stub docstrings; names are the fallback.
 */
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const STUBS = path.resolve(here, "../../knot-vscode/stubs/knot");
const OUT = path.resolve(here, "../web/src/js/pages/knotCompletions.js");

// ── .pyi parsing (simplified from generate-scriptling-completions.mjs) ─────

function walk(dir) {
  const out = [];
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, e.name);
    if (e.isDirectory()) out.push(...walk(p));
    else if (e.name.endsWith(".pyi")) out.push(p);
  }
  return out;
}

function moduleName(stubPath) {
  let rel = path.relative(STUBS, stubPath).replace(/\.pyi$/, "");
  rel = rel.split(path.sep).join("/");
  rel = rel.replace(/\/__init__$/, "");
  return rel.split("/").join(".");
}

function parseStub(content) {
  const functions = [];

  // Extract module docstring
  const docMatch = content.match(/^"""([\s\S]*?)"""/m);
  const moduleDoc = docMatch ? docMatch[1].trim().split("\n")[0] : "";

  // Match all function definitions. Params carry no nested parens in these
  // stubs and the return annotation ends at the ":" of the def line — a
  // docstring (and the "...") follows separately, so never consume into it.
  const fnRegex = /def\s+(\w+)\s*\(([^()]*)\)\s*->\s*([^\n:]+?)\s*:/g;
  let match;
  while ((match = fnRegex.exec(content)) !== null) {
    // Look ahead from the end of this match for a docstring on the
    // following lines (the injection puts it right after the def).
    const afterEnd = content.slice(match.index + match[0].length, match.index + match[0].length + 500);
    const docMatch2 = afterEnd.match(/^\s*"""([\s\S]*?)"""/);
    const docstring = docMatch2 ? docMatch2[1].trim() : "";

    functions.push({
      name: match[1],
      params: match[2].replace(/\s+/g, " ").trim(),
      returns: match[3].trim(),
      docstring,
    });
  }

  // Classes: a "class Name:" block up to the next top-level statement.
  // Methods parse with the same regex; annotated fields are collected as
  // documented members too.
  const classes = [];
  const classRegex = /^class\s+(\w+)\s*[(:]/gm;
  let classMatch;
  while ((classMatch = classRegex.exec(content)) !== null) {
    const bodyStart = content.indexOf("\n", classMatch.index) + 1;
    let bodyEnd = content.length;
    const nextTop = /^(?:class\s+\w+|def\s+\w+|""")/gm;
    nextTop.lastIndex = bodyStart;
    let m;
    while ((m = nextTop.exec(content)) !== null) {
      if (m.index > bodyStart) {
        bodyEnd = m.index;
        break;
      }
    }
    const body = content.slice(bodyStart, bodyEnd);
    const classDocMatch = body.match(/^\s*"""([\s\S]*?)"""/);
    const methods = [];
    const fieldRegex = /^\s+(\w+)\s*:\s*([^\n=]+)$/gm;
    let fm;
    while ((fm = fieldRegex.exec(body)) !== null) {
      if (fm[1] === "self") continue;
      methods.push({ name: fm[1], params: "", returns: fm[2].trim(), docstring: "" });
    }
    const methodRegex = /def\s+(\w+)\s*\(([^()]*)\)\s*->\s*([^\n:]+?)\s*:/g;
    let mm;
    while ((mm = methodRegex.exec(body)) !== null) {
      const after = body.slice(mm.index + mm[0].length, mm.index + mm[0].length + 500);
      const dm = after.match(/^\s*"""([\s\S]*?)"""/);
      methods.push({ name: mm[1], params: mm[2].replace(/\s+/g, " ").trim(), returns: mm[3].trim(), docstring: dm ? dm[1].trim() : "" });
    }
    classes.push({ name: classMatch[1], docstring: classDocMatch ? classDocMatch[1].trim() : "", methods });
  }

  // Module-level typed constants (NAME: type) — offered as completions
  // like functions, since editors complete both after "module.".
  const constants = [];
  const constRegex = /^(?:[A-Z][A-Z0-9_]*):\s*([A-Za-z[\]].+)$/gm;
  let constMatch;
  while ((constMatch = constRegex.exec(content)) !== null) {
    constants.push({ name: constMatch[0].split(":")[0].trim(), type: constMatch[1].trim() });
  }

  return { moduleDoc, functions, classes, constants };
}

// ── Description generation from function names ──────────────────────────────

function describeFunction(name, params) {
  // Convert snake_case to sentence
  const words = name.split("_");
  const verb = words[0];
  const object = words.slice(1).join(" ");
  const sentences = {
    get: `Get ${object || "value"}`,
    set: `Set ${object || "value"}`,
    list: `List ${object || "items"}`,
    create: `Create ${object || "item"}`,
    delete: `Delete ${object || "item"}`,
    update: `Update ${object || "item"}`,
    start: `Start ${object || "item"}`,
    stop: `Stop ${object || "item"}`,
    restart: `Restart ${object || "item"}`,
    is_: `Check if ${object || "condition"}`,
    wait: `Wait for ${object || "condition"}`,
    run: `Run ${object || "operation"}`,
    read: `Read ${object || "data"}`,
    write: `Write ${object || "data"}`,
    import: `Import ${object || "data"}`,
    export: `Export ${object || "data"}`,
  };
  const prefix = sentences[verb] || verb.charAt(0).toUpperCase() + verb.slice(1) + " " + (object || "");
  return prefix.charAt(0).toUpperCase() + prefix.slice(1);
}

function formatReturnType(pyType) {
  const mapping = {
    bool: "bool",
    str: "string",
    int: "int",
    float: "float",
    "dict[str, Any]": "dict",
    "list[dict[str, Any]]": "list of dicts",
    "list[str]": "list of strings",
    None: "None",
  };
  return mapping[pyType] || pyType.replace(/ Any/g, "").replace(/dict\[/, "dict<").replace(/\]/, ">").trim();
}

// ── Generation ───────────────────────────────────────────────────────────────

function generate() {
  if (!fs.existsSync(STUBS)) {
    console.error(`../knot-vscode/stubs/knot not found — skipping knot completions`);
    process.exit(1);
  }

  const stubs = walk(STUBS);
  const libraries = [];

  for (const stubPath of stubs) {
    const mod = moduleName(stubPath);
    const content = fs.readFileSync(stubPath, "utf-8");
    const { moduleDoc, functions, classes, constants } = parseStub(content);

    const entries = functions
      .filter((f) => !f.name.startsWith("_"))
      .map((f) => ({
        name: f.name,
        signature: formatSignature(f),
        description: f.docstring || describeFunction(f.name),
        returns: formatReturnType(f.returns),
      }));

    for (const c of constants || []) {
      entries.push({
        name: c.name,
        signature: c.name,
        description: `Constant (${c.type})`,
        returns: c.type,
      });
    }

    const classEntries = (classes || [])
      .map((c) => ({
        name: c.name,
        description: c.docstring,
        methods: c.methods.map((m) => ({
          name: m.name,
          signature: m.params ? `${m.name}(${m.params})` : m.name,
          description: m.docstring || describeFunction(m.name),
          returns: formatReturnType(m.returns),
        })),
      }));

    if (entries.length > 0 || classEntries.length > 0) {
      libraries.push({
        module: "knot." + mod,
        description: moduleDoc || `knot ${mod} library`,
        functions: entries,
        ...(classEntries.length > 0 ? { classes: classEntries } : {}),
      });
    }
  }

  const js = `// AUTO-GENERATED from ../knot-vscode/stubs/knot — do not edit by hand.
// Regenerate with: task scriptling-completions
// Descriptions come from the stub docstrings; names are the fallback.

export const knotLibraries = ${JSON.stringify(libraries, null, 2)};
`;

  fs.writeFileSync(OUT, js);
  console.log(`Generated ${OUT} (${libraries.length} libraries, ${libraries.reduce((a, l) => a + l.functions.length, 0)} functions)`);
}

function formatSignature(f) {
  // Split params on commas at bracket depth zero — annotations like
  // dict[str, str] carry commas of their own.
  const parts = [];
  let depth = 0;
  let current = "";
  for (const ch of f.params) {
    if (ch === "(" || ch === "[") depth += 1;
    else if (ch === ")" || ch === "]") depth -= 1;
    if (ch === "," && depth === 0) {
      parts.push(current);
      current = "";
      continue;
    }
    current += ch;
  }
  if (current.trim()) parts.push(current);
  const params = parts
    .map((p) => {
      const m = p.trim().match(/^(\w+)/);
      return m ? m[1] : p.trim();
    })
    .filter((p) => p && p !== "self");
  return `${f.name}(${params.join(", ")})`;
}

generate();
