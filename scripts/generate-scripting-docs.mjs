#!/usr/bin/env node
// Seeds the library pages of the AI scripting docs bundle
// (internal/scriptdocs/okf/knot-scripting/libraries/*.md) from the website's
// library reference (knot-website/content/reference/libraries/*.md),
// converting the Hugo frontmatter to OKF 0.2 frontmatter.
//
// Usage: node scripts/generate-scripting-docs.mjs [path/to/knot-website] [library ...]
//
// With no library names, every library AI-written scripts may import is
// converted (LIBRARIES below, which must match aiScriptLibraries in
// internal/service/ai_script.go). Pages are overwritten: once a page has
// been edited by hand, convert only the libraries you name.
//
// The conversion is deliberately simple: the frontmatter is rebuilt, the
// "Execution Environment" section (about where a library runs, which in an
// AI script is always the same) is dropped, and site-relative links are made
// absolute.

import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const LIBRARIES = [
  "apiclient", "audit", "event_sink", "files", "group", "identity", "jobs", "permission",
  "pool", "role", "script", "server", "skill", "slash_command", "space",
  "stack", "template", "tunnel", "user", "vars", "volume",
];

const here = path.dirname(fileURLToPath(import.meta.url));
const repo = path.resolve(here, "..");
const website = path.resolve(process.argv[2] || path.join(repo, "..", "knot-website"));
const names = process.argv.length > 3 ? process.argv.slice(3) : LIBRARIES;
const source = path.join(website, "content", "reference", "libraries");
const target = path.join(repo, "internal", "scriptdocs", "okf", "knot-scripting", "libraries");

function parseFrontmatter(text) {
  const m = text.match(/^---\r?\n([\s\S]*?)\r?\n---\r?\n?/);
  if (!m) return { meta: {}, body: text };
  const meta = {};
  for (const line of m[1].split(/\r?\n/)) {
    const kv = line.match(/^([A-Za-z_]+):\s*(.*)$/);
    if (!kv) continue;
    let value = kv[2].trim();
    if (value.startsWith("[") && value.endsWith("]")) {
      value = value.slice(1, -1).split(",").map((s) => s.trim()).filter(Boolean);
    } else if (/^".*"$/.test(value) || /^'.*'$/.test(value)) {
      value = value.slice(1, -1);
    }
    meta[kv[1]] = value;
  }
  return { meta, body: text.slice(m[0].length) };
}

function yamlString(s) {
  return JSON.stringify(String(s));
}

function dropSection(body, heading) {
  const lines = body.split("\n");
  const out = [];
  let skipping = false;
  for (const line of lines) {
    if (line.trim() === heading) {
      skipping = true;
      continue;
    }
    if (skipping && (line.startsWith("## ") || line.trim() === "---")) {
      skipping = false;
      if (line.trim() === "---") continue;
    }
    if (!skipping) out.push(line);
  }
  return out.join("\n");
}

fs.mkdirSync(target, { recursive: true });
let converted = 0;
for (const name of names) {
  const file = path.join(source, name + ".md");
  if (!fs.existsSync(file)) {
    console.error(`missing: ${file}`);
    process.exitCode = 1;
    continue;
  }
  const { meta, body } = parseFrontmatter(fs.readFileSync(file, "utf8"));
  const title = meta.title || `knot.${name}`;
  const tags = Array.isArray(meta.tags) ? meta.tags : [];
  let text = dropSection(body, "## Execution Environment");
  text = text.replace(/\]\(\/(?!\/)/g, "](https://getknot.dev/");
  text = text.replace(/\n{3,}/g, "\n\n").trim();

  const front = [
    "---",
    `type: ${yamlString(meta.type || "API Reference")}`,
    `title: ${yamlString(title)}`,
    `description: ${yamlString(meta.description || "")}`,
    `resource: ${yamlString(`https://getknot.dev/reference/libraries/${name}/`)}`,
    "tags:",
    ...tags.map((t) => `  - ${yamlString(t)}`),
    "status: stable",
    "generated:",
    "  by: knot/scripts/generate-scripting-docs.mjs",
    "sources:",
    `  - resource: ${yamlString(`https://getknot.dev/reference/libraries/${name}/`)}`,
    "---",
    "",
  ].join("\n");
  fs.writeFileSync(path.join(target, name + ".md"), `${front}# ${title}\n\n${text.replace(/^# .*\n+/, "")}\n`);
  converted++;
}
console.log(`converted ${converted} page(s) into ${path.relative(repo, target)}`);
