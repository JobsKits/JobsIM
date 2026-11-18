// Created by Jobs on 2026年11月17日，星期二.
// Evidence adapter: executes the existing counter and reads registrations; no build or service.
import fs from "node:fs";
import path from "node:path";
import crypto from "node:crypto";
import vm from "node:vm";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";

const evidence = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(evidence, "../../..");
const stats = path.resolve(evidence, "..");
const hash = (bytes) => crypto.createHash("sha256").update(bytes).digest("hex");
const read = (relative) => fs.readFileSync(path.join(root, relative));
const save = (name, value) => fs.writeFileSync(path.join(evidence, name), JSON.stringify(value, null, 2) + "\n", { flag: "wx" });
const stamp = new Date().toISOString().replaceAll(":", "").replaceAll(".", "");
const tool = "工具/统计项目.mjs/统计项目.mjs";
const toolSource = read(tool).toString("utf8");
const rules = (name) => {
  const expression = toolSource.match(new RegExp("const " + name + " = new Set\\((\\[[\\s\\S]*?\\])\\);"));
  if (!expression) throw Error("Existing counter rules changed: " + name);
  return vm.runInNewContext("new Set(" + expression[1] + ")", { Set });
};
const excluded = rules("excluded"), business = rules("businessExtensions"), configuration = rules("configurationExtensions"), names = rules("configurationNames");

function inputs() {
  const result = {};
  function visit(relative) {
    for (const entry of fs.readdirSync(path.join(root, relative), { withFileTypes: true })) {
      if (entry.isSymbolicLink() || excluded.has(entry.name)) continue;
      const target = relative + "/" + entry.name;
      if (entry.isDirectory()) { visit(target); continue; }
      if (/(?:-lock\.json|\.lock|\.sum)$|\.xcassets\//i.test(target) || entry.name === "报告.json") continue;
      const extension = path.extname(entry.name);
      if (business.has(extension) || configuration.has(extension) || names.has(entry.name)) result[target] = hash(read(target));
    }
  }
  for (const directory of ["代码", "测试", "部署", "工具"]) visit(directory);
  return Object.fromEntries(Object.entries(result).sort(([a], [b]) => a.localeCompare(b)));
}

const before = inputs();
save("01-统计输入前.json", { at: new Date().toISOString(), counterSHA256: hash(read(tool)), files: before });
const history = path.join(stats, "历史");
fs.mkdirSync(history, { recursive: true });
const backups = [];
for (const relative of ["项目相关报告/项目统计/报告.json", "项目相关报告/项目统计/HTTP路由源码快照.json", "项目相关报告/项目统计/README.md", "README.md"]) {
  const bytes = read(relative), extension = relative.endsWith(".md") ? ".md.txt" : ".json";
  const destination = path.join(history, "20261117-" + stamp + "-读取前-" + path.basename(relative, path.extname(relative)) + extension);
  if (relative === "README.md") {
    const unique = destination.replace("-README.md.txt", "-根README.md.txt");
    fs.writeFileSync(unique, bytes, { flag: "wx" }); backups.push({ source: relative, path: path.relative(stats, unique), sha256: hash(bytes) });
  } else {
    fs.writeFileSync(destination, bytes, { flag: "wx" }); backups.push({ source: relative, path: path.relative(stats, destination), sha256: hash(bytes) });
  }
}
save("02-历史保全.json", backups);
const run = spawnSync(process.execPath, [path.join(root, tool)], { cwd: root, encoding: "utf8", timeout: 15000, maxBuffer: 1024 * 1024 });
save("03-原统计工具执行.json", { startedAt: stamp, status: run.status, signal: run.signal, stdout: run.stdout, stderr: run.stderr, error: run.error?.message });
if (run.status !== 0) throw Error("Original counter failed; history preserved");
const report = JSON.parse(read("项目相关报告/项目统计/报告.json"));
const after = inputs(), changes = [...new Set([...Object.keys(before), ...Object.keys(after)])].filter((file) => before[file] !== after[file]);
const outputNames = report.files.map((entry) => entry.path).sort();
const sameNames = JSON.stringify(outputNames) === JSON.stringify(Object.keys(after).sort());
save("04-统计输入后.json", { at: new Date().toISOString(), files: after, changed: changes, sameReportFileList: sameNames });
if (changes.length || !sameNames) throw Error("Concurrent source drift or inventory mismatch; do not publish these numbers");

const goFiles = Object.keys(after).filter((file) => file.startsWith("代码/后端/Go/") && file.endsWith(".go") && !file.endsWith("_test.go"));
const definitions = new Map();
for (const file of goFiles.filter((file) => path.dirname(file) === "代码/后端/Go")) {
  const source = read(file).toString("utf8"), methods = [...source.matchAll(/^func \(a \*App\) (\w+)\(/gm)];
  for (let index = 0; index < methods.length; index++) {
    const match = methods[index];
    definitions.set(match[1], { file, start: match.index, text: source.slice(match.index, methods[index + 1]?.index ?? source.length), source });
  }
}
const expansions = {
  selfReadRoutes: { expression: "pattern", required: ["range []bool{false, true}", "PUT /api/conversations/{id}/read", "POST /api/conversations/{id}/read-results"], patterns: ["PUT /api/conversations/{id}/read", "POST /api/conversations/{id}/read-results"] },
  selfViewRoutes: { expression: "pattern", required: ["range []bool{false, true}", "PATCH /api/conversations/{id}/view", "POST /api/conversations/{id}/view-results"], patterns: ["PATCH /api/conversations/{id}/view", "POST /api/conversations/{id}/view-results"] },
  selfDraftRoutes: { expression: 'method+" /api/conversations/{id}/draft"', required: ['range []string{"PUT", "DELETE"}'], patterns: ["PUT /api/conversations/{id}/draft", "DELETE /api/conversations/{id}/draft"] },
};
const internalDynamic = {
  draftReplyHolderRoutes: { expression: '"POST /internal/drafts/"+action+"-reply"', required: ['range []string{"promote", "release"}'] },
  forwardRevocationRoutes: { expression: '"POST "+path', required: ['range []string{"/internal/forwards/revoke-principal", "/internal/forwards/revoke-membership", "/internal/forwards/revoke-source"}'] },
  exportMessageRoutes: { expression: '"POST "+endpoint', required: ['range []string{"/internal/exports/read-message", "/internal/exports/check-message"}'] },
  mfaRoutes: { expression: '"POST "+endpoint', required: ['range []string{"/internal/step-up/check", "/internal/step-up/consume"}'] },
  personalExportOwnerRoutes: { expression: '"POST /internal/personal-exports/"+path', required: ['range []string{"capture", "register", "read", "check", "check-avatar", "clear-holders"}'] },
  syncOwnerRoutes: { expression: '"POST /internal/sync/"+p', required: ['range []string{"capture", "watermark", "events", "hydrate"}'] },
};
const registrations = [], dynamicEvidence = [], excludedInternal = [], unresolved = [];
const validateEvidence = (definition, rule) => rule.required.every((literal) => definition.text.includes(literal));
for (const [name, definition] of definitions) {
  for (const match of definition.text.matchAll(/^\s*(?:h|handle)\(([^,\n]+),/gm)) {
    const expression = match[1].trim(), location = { file: definition.file, function: name, line: definition.source.slice(0, definition.start + match.index).split("\n").length, expression };
    let patterns;
    if (/^"(?:[^"\\]|\\.)*"$/.test(expression)) patterns = [JSON.parse(expression)];
    else if (expansions[name]?.expression === expression && validateEvidence(definition, expansions[name])) patterns = expansions[name].patterns;
    else if (name === "messageRoutes" && /^"(PUT|DELETE) \/api\/messages\/\{id\}\/"\+k$/.test(expression) && definition.text.includes('range []string{"pin", "favorite"}')) patterns = ["pin", "favorite"].map((kind) => expression.match(/^"(PUT|DELETE)/)[1] + " /api/messages/{id}/" + kind);
    else if (internalDynamic[name]?.expression === expression && validateEvidence(definition, internalDynamic[name])) { excludedInternal.push(location); continue; }
    else { unresolved.push(location); continue; }
    if (!/^"(?:[^"\\]|\\.)*"$/.test(expression)) dynamicEvidence.push({ ...location, patterns });
    for (const pattern of patterns) if (/^[A-Z]+ \/api\//.test(pattern)) registrations.push({ pattern, ...location });
  }
}
const reached = new Map([["Handler", ["Handler"]]]);
for (let changed = true; changed;) {
  changed = false;
  for (const [name, chain] of [...reached]) for (const match of (definitions.get(name)?.text ?? "").matchAll(/a\.(\w+)\((?:h|handle)(?:,|\))/g)) {
    if (!reached.has(match[1])) { reached.set(match[1], [...chain, match[1]]); changed = true; }
  }
}
const unreachable = registrations.filter((registration) => !reached.has(registration.function));
save("05-公开注册静态依据.json", { algorithm: "Literal h/handle registrations in App methods; reviewed bounded public loop expansions; registration call paths from App.Handler; no Go execution", dynamicEvidence, excludedInternalDynamic: excludedInternal, unresolved, unreachable });
if (unresolved.length || unreachable.length) throw Error("Unresolved registration: mark static pending instead of inventing a count");
const unique = [...new Map(registrations.map((registration) => [registration.pattern, registration])).values()];
const routes = { ordinary: [], admin: [], experimental: [], health: [] }, evidenceRows = [];
for (const registration of unique) {
  const pattern = registration.pattern;
  const category = pattern === "GET /api/health" ? "health" : registration.function === "exportRoutes" ? "experimental" : pattern.includes(" /api/admin/") || ["POST /api/auth/step-up", "PUT /api/capabilities/{name}"].includes(pattern) ? "admin" : "ordinary";
  routes[category].push(pattern); evidenceRows.push({ ...registration, category, callPath: reached.get(registration.function) });
}
for (const list of Object.values(routes)) list.sort();
const previous = JSON.parse(fs.readFileSync(path.join(stats, backups.find((entry) => entry.source.endsWith("HTTP路由源码快照.json")).path)));
const previousPatterns = new Set(Object.values(previous.routes).flat()), currentPatterns = new Set(unique.map((entry) => entry.pattern));
const finalInputs = inputs(), finalChanges = Object.keys(after).filter((file) => after[file] !== finalInputs[file]);
if (finalChanges.length || Object.keys(after).length !== Object.keys(finalInputs).length) throw Error("Sources drifted during HTTP review; retain evidence, do not publish");
const snapshot = {
  at: new Date().toISOString(), method: "Static source registration inventory of unique METHOD + /api path patterns; GET-implied HEAD, gateway proxies, Web static/fallback handlers and /internal service RPC excluded. Not runtime ServeMux enumeration or HTTPS coverage.",
  currentProductionGoFiles: goFiles.length, sourceSHA256: Object.fromEntries(goFiles.map((file) => [file, after[file]])),
  counts: Object.fromEntries(Object.entries(routes).map(([category, list]) => [category, list.length])), totalExperimentalOn: unique.length,
  totalExperimentalOff: unique.length - routes.experimental.length, runtimeExhaustive: false, staticEnumerationCompleteForReviewedRegistrationForms: true,
  classification: { admin: "All /api/admin routes plus POST /api/auth/step-up and PUT /api/capabilities/{name}; excluded from ordinary metric", experimental: "exportRoutes called only under Config.ExperimentalExports in Handler; default false", health: "GET /api/health counted once across roles", ordinary: "Remaining public business registration patterns, including feature-gated routes; not a deployed or tested-coverage claim" },
  routes, registrationEvidence: evidenceRows.sort((a, b) => a.pattern.localeCompare(b.pattern)), dynamicEvidence, excludedInternalDynamic: excludedInternal,
  previousSnapshot: backups.find((entry) => entry.source.endsWith("HTTP路由源码快照.json")), addedSincePrevious: [...currentPatterns].filter((pattern) => !previousPatterns.has(pattern)).sort(), removedSincePrevious: [...previousPatterns].filter((pattern) => !currentPatterns.has(pattern)).sort(),
};
fs.writeFileSync(path.join(stats, "HTTP路由源码快照.json"), JSON.stringify(snapshot, null, 2) + "\n");
const controllers = Object.keys(after).filter((file) => file.startsWith("代码/前端/iOS/JobsIM/UI/") && file.endsWith(".swift")).flatMap((file) => [...read(file).toString("utf8").matchAll(/final class (\w+): IMBaseVC/g)].map((match) => ({ file, class: match[1] })));
save("06-读取结果.json", { at: new Date().toISOString(), counterSHA256: hash(read(tool)), statistics: { at: report.at, totals: report.totals, totalLines: report.totalLines }, sourceFileCount: Object.keys(after).length, changedSources: finalChanges, sameReportFileList: sameNames, HTTP: { sourceFiles: goFiles.length, counts: snapshot.counts, totalExperimentalOn: snapshot.totalExperimentalOn, totalExperimentalOff: snapshot.totalExperimentalOff }, iosConcreteControllers: controllers, history: backups, build: false, service: false, stageOrCommit: false });
console.log(JSON.stringify({ evidence: path.relative(root, evidence), totals: report.totals, totalLines: report.totalLines, HTTP: snapshot.counts, productionGoFiles: goFiles.length, iosControllers: controllers.length, changedSources: finalChanges }));
