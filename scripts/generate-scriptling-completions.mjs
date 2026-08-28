#!/usr/bin/env node
/**
 * Generate the ACE editor completion data for Scriptling libraries from the
 * type stubs in ../scriptling-vscode (the IntelliSense source of truth).
 *
 * Usage:   node scripts/generate-scriptling-completions.mjs
 * Output:  web/src/js/pages/scriptlingCompletions.js
 *
 * The generated file is imported by scriptCompletions.js, which keeps only
 * the knot.* entries of its own. Re-run this whenever the vscode stubs change.
 */
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const STUBS = path.resolve(here, "../../scriptling-vscode/stubs");
const OUT = path.resolve(here, "../web/src/js/pages/scriptlingCompletions.js");

// ── .pyi parsing ─────────────────────────────────────────────────────────────

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

function splitTop(s) {
  const parts = [];
  let depth = 0,
    cur = "",
    q = null;
  for (let i = 0; i < s.length; i++) {
    const c = s[i];
    if (q) {
      cur += c;
      if (c === q && s[i - 1] !== "\\") q = null;
      continue;
    }
    if (c === '"' || c === "'") {
      q = c;
      cur += c;
      continue;
    }
    if (c === "(" || c === "[" || c === "{") depth++;
    if (c === ")" || c === "]" || c === "}") depth--;
    if (c === "," && depth === 0) {
      parts.push(cur);
      cur = "";
      continue;
    }
    cur += c;
  }
  if (cur.trim()) parts.push(cur);
  return parts;
}

function collapse(s) {
  return s.replace(/\s+/g, " ").trim();
}

function dedentBlock(text) {
  const lines = text.split("\n");
  const indents = lines
    .filter((l) => l.trim())
    .map((l) => l.match(/^\s*/)[0].length);
  const min = indents.length ? Math.min(...indents) : 0;
  return lines.map((l) => l.slice(min)).join("\n");
}

function docParts(raw) {
  if (!raw) return { description: "", returnsDesc: "" };
  const text = dedentBlock(raw);
  const paragraphs = text.split(/\n\s*\n/);
  const description = collapse(paragraphs[0] || "");
  let returnsDesc = "";
  const lines = text.split("\n");
  const idx = lines.findIndex((l) => /^Returns:?\s*$/.test(l.trim()));
  if (idx >= 0) {
    const ret = [];
    for (let j = idx + 1; j < lines.length; j++) {
      const l = lines[j];
      if (l.trim() && !/^\s/.test(l)) break;
      if (l.trim()) ret.push(l.trim());
      else if (ret.length) break;
    }
    returnsDesc = collapse(ret.join(" "));
  }
  return { description, returnsDesc };
}

class Parser {
  constructor(src) {
    this.lines = src.split("\n");
    this.i = 0;
  }

  peek() {
    while (
      this.i < this.lines.length &&
      (this.lines[this.i].trim() === "" ||
        this.lines[this.i].trim().startsWith("#"))
    ) {
      this.i++;
    }
    return this.i < this.lines.length ? this.lines[this.i] : null;
  }

  lineIndent() {
    const l = this.peek();
    return l ? l.match(/^\s*/)[0].length : -1;
  }

  readDocstring() {
    const l = this.peek();
    if (!l) return "";
    const t = l.trim();
    if (!t.startsWith('"""')) return "";
    // Single-line docstring
    if (t.length > 3 && t.endsWith('"""')) {
      this.i++;
      return t.slice(3, -3);
    }
    const parts = [t.slice(3)];
    this.i++;
    while (this.i < this.lines.length) {
      const line = this.lines[this.i++];
      const end = line.indexOf('"""');
      if (end >= 0) {
        parts.push(line.slice(0, end));
        break;
      }
      parts.push(line);
    }
    return parts.join("\n");
  }

  readDef() {
    const startIndent = this.lineIndent();
    let header = this.lines[this.i++].trim();
    // Join continuation lines until parentheses balance.
    while ((header.match(/\(/g) || []).length > (header.match(/\)/g) || []).length) {
      header += " " + this.lines[this.i++].trim();
    }
    const m = header.match(/^def\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(([\s\S]*)\)(?:\s*->\s*(.+?))?:$/);
    if (!m) return null;
    const [, name, params, retAnn] = m;

    const doc = this.readDocstring();
    // Skip the rest of the body (docstring "...", statements) until dedent.
    while (this.i < this.lines.length) {
      const l = this.lines[this.i];
      if (l.trim() === "") {
        this.i++;
        continue;
      }
      if (l.match(/^\s*/)[0].length > startIndent) this.i++;
      else break;
    }

    const { description, returnsDesc } = docParts(doc);
    const displayParams = splitTop(params)
      .map((p) => {
        const pm = p
          .trim()
          .match(/^(\*{0,2}[A-Za-z_][A-Za-z0-9_]*)\s*(?::[^=]+)?(?:=([\s\S]*))?$/);
        if (!pm) return p.trim();
        return pm[2] !== undefined ? `${pm[1]}=${pm[2].trim()}` : pm[1];
      })
      .filter((p) => p !== "self" && p !== "cls" && p !== "*");

    let retType = retAnn ? retAnn.trim() : "";
    retType = retType.replace(/^Optional\[(.*)\]$/, "$1").trim();

    const entry = {
      name,
      signature: `${name}(${displayParams.join(", ")})`,
      description,
    };
    if (retType) entry.returns = returnsDesc ? `${retType} - ${returnsDesc}` : retType;
    else if (returnsDesc) entry.returns = returnsDesc;
    return entry;
  }

  readClass() {
    const startIndent = this.lineIndent();
    const header = this.lines[this.i++].trim();
    const name = header.match(/^class\s+([A-Za-z_][A-Za-z0-9_]*)/)[1];
    const cls = { name, description: "", methods: [], properties: [] };

    // One-line class (e.g. class FooError(Exception): ...) has no members.
    if (header.endsWith("...")) {
      const doc = this.readDocstring();
      if (doc) cls.description = docParts(doc).description;
      return cls;
    }

    const doc = this.readDocstring();
    if (doc) cls.description = docParts(doc).description;

    const methods = new Map();
    while (this.i < this.lines.length) {
      const l = this.lines[this.i];
      if (l.trim() === "") {
        this.i++;
        continue;
      }
      if (l.match(/^\s*/)[0].length <= startIndent) break;

      const t = l.trim();
      if (t.startsWith("@") || t.startsWith("#")) {
        this.i++;
        continue;
      }
      if (t.startsWith("def ")) {
        const def = this.readDef();
        if (def) methods.set(def.name, def);
        continue;
      }
      const attr = t.match(/^([A-Za-z_][A-Za-z0-9_]*)\s*:\s*(.+?)(?:\s*=\s*.*)?$/);
      if (attr) {
        cls.properties.push({ name: attr[1], description: attr[2].trim() });
        this.i++;
        continue;
      }
      this.i++;
    }
    cls.methods = [...methods.values()];
    return cls;
  }

  parseModule() {
    const mod = { description: "", functions: [], classes: [], constants: [] };
    const doc = this.readDocstring();
    mod.description = doc ? docParts(doc).description : "";

    const functions = new Map();
    const classes = new Map();

    while (this.i < this.lines.length) {
      const l = this.lines[this.i];
      const t = l.trim();
      if (
        t === "" ||
        t.startsWith("#") ||
        t.startsWith("import ") ||
        t.startsWith("from ") ||
        t.startsWith("@") ||
        t.startsWith("if TYPE_CHECKING")
      ) {
        this.i++;
        continue;
      }
      if (t.startsWith("def ")) {
        const def = this.readDef();
        if (def) functions.set(def.name, def);
        continue;
      }
      if (t.startsWith("class ")) {
        const cls = this.readClass();
        if (cls) classes.set(cls.name, cls);
        continue;
      }
      const const_ = t.match(
        /^([A-Z][A-Z0-9_]*)\s*(?::\s*([^=]+?))?\s*(?:=\s*(.+?))?$/
      );
      if (const_ && !(const_[3] || "").includes("TypeVar(")) {
        mod.constants.push({
          name: const_[1],
          value: const_[3] !== undefined ? const_[3].trim() : "",
          description: const_[2] ? const_[2].trim() : "",
        });
        this.i++;
        continue;
      }
      this.i++;
    }

    mod.functions = [...functions.values()];
    mod.classes = [...classes.values()];
    return mod;
  }
}

// ── code generation ──────────────────────────────────────────────────────────

function prop(name, value) {
  return value === undefined ? "" : ` ${name}: ${JSON.stringify(value)},`;
}

function emitFunction(fn, indent) {
  const pad = " ".repeat(indent);
  let s = `${pad}{\n`;
  s += `${pad}  name: ${JSON.stringify(fn.name)},\n`;
  s += `${pad}  signature: ${JSON.stringify(fn.signature)},\n`;
  s += `${pad}  description: ${JSON.stringify(fn.description)},\n`;
  if (fn.returns !== undefined)
    s += `${pad}  returns: ${JSON.stringify(fn.returns)},\n`;
  s += `${pad}},\n`;
  return s;
}

function emitClass(cls, indent) {
  const pad = " ".repeat(indent);
  let s = `${pad}{\n`;
  s += `${pad}  name: ${JSON.stringify(cls.name)},\n`;
  s += `${pad}  description: ${JSON.stringify(cls.description)},\n`;
  if (cls.methods.length) {
    s += `${pad}  methods: [\n`;
    for (const m of cls.methods) s += emitFunction(m, indent + 4);
    s += `${pad}  ],\n`;
  }
  if (cls.properties.length) {
    s += `${pad}  properties: [\n`;
    for (const p of cls.properties) {
      s += `${pad}  {\n`;
      s += `${pad}    name: ${JSON.stringify(p.name)},\n`;
      s += `${pad}    description: ${JSON.stringify(p.description)},\n`;
      s += `${pad}  },\n`;
    }
    s += `${pad}  ],\n`;
  }
  s += `${pad}},\n`;
  return s;
}

function emitModule(mod) {
  let s = "  {\n";
  s += `    module: ${JSON.stringify(mod.module)},\n`;
  s += `    description: ${JSON.stringify(mod.description)},\n`;
  if (mod.functions.length) {
    s += "    functions: [\n";
    for (const fn of mod.functions) s += emitFunction(fn, 6);
    s += "    ],\n";
  }
  if (mod.classes.length) {
    s += "    classes: [\n";
    for (const cls of mod.classes) s += emitClass(cls, 6);
    s += "    ],\n";
  }
  if (mod.constants.length) {
    s += "    constants: [\n";
    for (const c of mod.constants) {
      s += "      {\n";
      s += `        name: ${JSON.stringify(c.name)},\n`;
      if (c.value) s += `        value: ${JSON.stringify(c.value)},\n`;
      if (c.description)
        s += `        description: ${JSON.stringify(c.description)},\n`;
      s += "      },\n";
    }
    s += "    ],\n";
  }
  s += "  },\n";
  return s;
}

// ── main ─────────────────────────────────────────────────────────────────────

const files = walk(STUBS).sort();
const modules = files.map((f) => {
  const mod = new Parser(fs.readFileSync(f, "utf8")).parseModule();
  mod.module = moduleName(f);
  return mod;
});
modules.sort((a, b) => a.module.localeCompare(b.module));

let out = `// GENERATED FILE — do not edit by hand.
// Source of truth: ../scriptling-vscode/stubs (the IntelliSense type stubs).
// Regenerate with: node scripts/generate-scriptling-completions.mjs
//                 (or: task scriptling-completions)
// Provides the scriptling library completions for the ACE editor (imported by
// scriptCompletions.js, which keeps the knot.* entries of its own).

const scriptlingLibraries = [
`;

for (const mod of modules) out += emitModule(mod);

out += `];

export { scriptlingLibraries };
`;

fs.mkdirSync(path.dirname(OUT), { recursive: true });
fs.writeFileSync(OUT, out);
console.log(
  `wrote ${OUT}: ${modules.length} modules, ` +
    `${modules.reduce((n, m) => n + m.functions.length, 0)} functions, ` +
    `${modules.reduce((n, m) => n + m.classes.length, 0)} classes`
);
