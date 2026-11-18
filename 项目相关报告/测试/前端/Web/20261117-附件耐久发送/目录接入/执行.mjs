import fs from "node:fs";
import path from "node:path";
import crypto from "node:crypto";
import { fileURLToPath, pathToFileURL } from "node:url";
import { spawnSync } from "node:child_process";

const here = path.dirname(fileURLToPath(import.meta.url)), root = path.resolve(here, "../../../../../..");
const { attributePattern } = await import(pathToFileURL(path.join(process.env.HOME, ".agents/skills/jobs-markdown-docs/scripts/deploy-markdown-toc.mjs")));
const { cleanMarkdown, smudgeMarkdown, inspectMarkdown, checkMarkdown } = await import(pathToFileURL(path.join(root, ".jobs-markdown-toc/MarkdownToc.mjs")));
const sha = bytes => crypto.createHash("sha256").update(bytes).digest("hex");
const save = (name, value) => fs.writeFileSync(path.join(here, name), JSON.stringify(value, null, 2) + "\n", { flag: "wx" });
const git = args => {
  const result = spawnSync("git", args, { cwd: root, env: { ...process.env, GIT_OPTIONAL_LOCKS: "0" }, maxBuffer: 16 * 1024 * 1024 });
  if (result.status !== 0) throw Error(result.stderr.toString());
  return result.stdout;
};
const run = (name, mode) => {
  const result = spawnSync(process.execPath, [".jobs-markdown-toc/MarkdownToc.mjs", mode], { cwd: root, env: { ...process.env, GIT_OPTIONAL_LOCKS: "0" }, encoding: "utf8", timeout: 60000, maxBuffer: 4 * 1024 * 1024 });
  const value = { at: new Date().toISOString(), mode, status: result.status, signal: result.signal, stdout: result.stdout, stderr: result.stderr, error: result.error?.message };
  save(name, value); return value;
};
const additions = ["测试/前端/Web/attachment-rules.cjs/README.md", "测试/前端/Web/attachment-browser.cjs/README.md", "项目相关报告/测试/前端/Web/20261117-附件耐久发送/README.md"];
const manifestPath = path.join(root, ".jobs-markdown-toc/manifest.json"), attrsPath = path.join(root, ".gitattributes");
const manifestBytes = fs.readFileSync(manifestPath), attrsBytes = fs.readFileSync(attrsPath), manifest = JSON.parse(manifestBytes);
if (manifest.version !== 1 || manifest.files.length !== 190 || additions.some(file => manifest.files.includes(file))) throw Error("Expected untouched 190-item baseline and three new entries");
const files = [...manifest.files, ...additions];
const docHashes = () => Object.fromEntries(files.map(file => [file, sha(fs.readFileSync(path.join(root, file)))]));
const index = () => ({ staged: sha(git(["ls-files", "--stage", "-z"])), flags: sha(git(["ls-files", "-v", "-z"])) });
const beforeDocs = docHashes(), beforeIndex = index();
save("01-执行前.json", { at: new Date().toISOString(), root, count: manifest.files.length, additions, manifestSHA: sha(manifestBytes), attrsSHA: sha(attrsBytes), converterSHA: sha(fs.readFileSync(path.join(root, ".jobs-markdown-toc/MarkdownToc.mjs"))), index: beforeIndex, documents: beforeDocs });
fs.writeFileSync(path.join(here, "原manifest.json"), manifestBytes, { flag: "wx" });
fs.writeFileSync(path.join(here, "原gitattributes.txt"), attrsBytes, { flag: "wx" });
const previews = [];
for (const file of additions) {
  const original = fs.readFileSync(path.join(root, file), "utf8"), generated = cleanMarkdown(original);
  if (!inspectMarkdown(original).tocCount || smudgeMarkdown(generated) !== original || !checkMarkdown(generated)) throw Error("Invalid source or round trip: " + file);
  previews.push({ file, sourceSHA: sha(original), generatedSHA: sha(generated), inspection: inspectMarkdown(original), checkGenerated: true });
}
save("02-三文档转换.json", previews);
const attrs = attrsBytes.toString("utf8"), marker = "# jobs-markdown-toc:end";
if (attrs.split(marker).length !== 2 || attrs.split("# jobs-markdown-toc:start").length !== 2) throw Error("Unexpected attribute block");
const lines = additions.map(file => attributePattern(file) + " filter=jobs-markdown-toc\n").join("");
const nextAttrs = attrs.replace(marker, lines + marker), nextManifest = JSON.stringify({ ...manifest, files }, null, 2) + "\n";
if (sha(fs.readFileSync(manifestPath)) !== sha(manifestBytes) || sha(fs.readFileSync(attrsPath)) !== sha(attrsBytes)) throw Error("Concurrent configuration edit; nothing written");
fs.writeFileSync(manifestPath, nextManifest); fs.writeFileSync(attrsPath, nextAttrs);
const input = files.join("\0") + "\0", attributes = spawnSync("git", ["check-attr", "-z", "--stdin", "filter"], { cwd: root, input, encoding: "utf8", env: { ...process.env, GIT_OPTIONAL_LOCKS: "0" } });
if (attributes.status !== 0) throw Error(attributes.stderr);
const attrRows = attributes.stdout.split("\0");
if (files.some((file, i) => attrRows[i * 3] !== file || attrRows[i * 3 + 1] !== "filter" || attrRows[i * 3 + 2] !== "jobs-markdown-toc")) throw Error("Effective attribute mismatch");
save("03-精确登记.json", { count: files.length, original190Preserved: files.slice(0, 190).every((file, i) => file === manifest.files[i]), additions, attributes: additions.map(file => attributePattern(file) + " filter=jobs-markdown-toc"), effectiveAttributeCount: files.length, originalAttributesBytePreserved: nextAttrs.replace(lines, "") === attrs });
const install = run("10-install.json", "install");
const verify = run("11-verify.json", "verify"), status = run("12-status.json", "status"), check = run("13-index-check.json", "check");
const afterDocs = docHashes(), afterIndex = index(), changedDocs = files.filter(file => beforeDocs[file] !== afterDocs[file]);
const finalManifest = JSON.parse(fs.readFileSync(manifestPath));
const result = { at: new Date().toISOString(), manifestCount: finalManifest.files.length, original190Preserved: finalManifest.files.slice(0, 190).every((file, i) => file === manifest.files[i]), addedExactly: JSON.stringify(finalManifest.files.slice(190)) === JSON.stringify(additions), installStatus: install.status, verifyStatus: verify.status, statusStatus: status.status, rawIndexCheckStatus: check.status, checkIsPublishValidationOnly: true, changedDocs, newThreeBodyUnchanged: additions.every(file => beforeDocs[file] === afterDocs[file]), all193BodyUnchanged: changedDocs.length === 0, stagedUnchanged: beforeIndex.staged === afterIndex.staged, flagsUnchanged: beforeIndex.flags === afterIndex.flags, beforeIndex, afterIndex, afterDocuments: afterDocs, noStageCommitPush: true, heavyTests: false };
save("14-交付结果.json", result); console.log(JSON.stringify({ ...result, afterDocuments: undefined }));
if (verify.status !== 0 || !result.original190Preserved || !result.addedExactly || !result.newThreeBodyUnchanged || !result.stagedUnchanged || !result.flagsUnchanged) process.exitCode = 1;
