#!/usr/bin/env node
// Created by Jobs on 2026年11月15日，星期日.
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(here, "../..");
const excluded = new Set([
  ".git",
  "node_modules",
  "Pods",
  "Vendor",
  "build",
  "Build",
  "Dist",
  "dist",
  "DerivedData",
  ".gradle",
  ".hvigor",
  "oh_modules",
  "报告",
  "Resource",
  "resources",
  "res",
  "Public",
  "第三方许可",
  "schemas",
]);
const businessExtensions = new Set([
  ".go",
  ".ts",
  ".tsx",
  ".css",
  ".swift",
  ".kt",
  ".ets",
]);
const configurationExtensions = new Set([
  ".mjs",
  ".cjs",
  ".rb",
  ".sh",
  ".command",
  ".ps1",
  ".bat",
  ".json",
  ".json5",
  ".yaml",
  ".yml",
  ".toml",
  ".podspec",
  ".plist",
  ".pbxproj",
  ".entitlements",
  ".mod",
  ".properties",
  ".kts",
  ".xml",
  ".xcconfig",
]);
const configurationNames = new Set([
  "Podfile",
  "Podfile.deps",
  "Gemfile",
  "Dockerfile",
]);
const totals = Object.fromEntries(
  ["business", "tests", "configuration"].map((key) => [
    key,
    { files: 0, lines: 0 },
  ]),
);
const files = [];

function visit(directory) {
  for (const entry of fs.readdirSync(directory, { withFileTypes: true })) {
    if (entry.isSymbolicLink() || excluded.has(entry.name)) continue;
    const target = path.join(directory, entry.name);
    if (entry.isDirectory()) {
      visit(target);
      continue;
    }
    const relative = path.relative(root, target).split(path.sep).join("/");
    if (
      /(?:-lock\.json|\.lock|\.sum)$|\.xcassets\//i.test(relative) ||
      entry.name === "报告.json"
    )
      continue;
    const extension = path.extname(entry.name);
    if (
      !businessExtensions.has(extension) &&
      !configurationExtensions.has(extension) &&
      !configurationNames.has(entry.name)
    )
      continue;
    let category = relative.startsWith("测试/")
      ? "tests"
      : businessExtensions.has(extension)
        ? "business"
        : "configuration";
    if (entry.name.endsWith(".gradle.kts")) category = "configuration";
    const text = fs.readFileSync(target, "utf8");
    const lines = text
      ? text.split("\n").length - Number(text.endsWith("\n"))
      : 0;
    totals[category].files++;
    totals[category].lines += lines;
    files.push({ path: relative, category, lines });
  }
}

for (const directory of ["代码", "测试", "部署", "工具"])
  visit(path.join(root, directory));
const totalLines = Object.values(totals).reduce(
  (total, value) => total + value.lines,
  0,
);
for (const value of Object.values(totals))
  value.percent = Number(((value.lines / totalLines) * 100).toFixed(2));
const result = {
  at: new Date().toISOString(),
  method:
    "Physical lines including comments and blank lines; owned code/test/config only; symlinks, dependencies, resources, locks, generated/build outputs and evidence excluded.",
  totals,
  totalLines,
  files,
};
fs.writeFileSync(
  path.join(here, "报告.json"),
  JSON.stringify(result, null, 2) + "\n",
);
console.log(JSON.stringify({ at: result.at, totals, totalLines }, null, 2));
