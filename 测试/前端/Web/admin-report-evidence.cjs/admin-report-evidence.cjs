// Created by Jobs on 2026年11月18日，星期三.
const fs = require("node:fs"), path = require("node:path"), vm = require("node:vm"), assert = require("node:assert/strict");
const root = path.resolve(__dirname, "../../../.."), web = path.join(root, "代码/前端/Web");
const { stripTypeScriptTypes } = require("node:module");
function load(name) {
  const source = fs.readFileSync(path.join(web, "Src", name), "utf8");
  const names = [...source.matchAll(/^export (?:function|const) (\w+)/gm)].map(match => match[1]);
  const output = stripTypeScriptTypes(source).replace(/^export /gm, "") + "\nObject.assign(exports,{" + names.join(",") + "});";
  const exports = {}; vm.runInNewContext(output, { exports, URL, URLSearchParams, Uint8Array, Set, Number, Error }); return exports;
}
const protocol = load("AdminReportProtocol.ts"), labels = load("AdminReportLabels.ts").adminReportLabels;
const reportId = "11111111-1111-4111-8111-111111111111", objectId = "22222222-2222-4222-8222-222222222222", messageId = "33333333-3333-4333-8333-333333333333", actorId = "44444444-4444-4444-8444-444444444444";
const evidenceURL = "/api/reports/evidence/" + objectId + "?reportId=" + reportId;
const messageURL = "/api/admin/reports/" + reportId + "/messages/" + messageId + "/attachment";
const report = { id: reportId, reporterId: actorId, targetId: actorId, targetType: "user", reason: "Selected material", state: "submitted", version: "1", createdAt: "2026-11-18T00:00:00Z", updatedAt: "2026-11-18T00:00:00Z" };
const detail = () => ({ report, evidence: [{ id: objectId, name: "selected.png", contentType: "image/png", size: 8, sha256: "a".repeat(64), url: evidenceURL, available: true }], selectedMessages: [{ id: messageId, conversationId: reportId, senderId: actorId, type: "text", text: "Only this selected message", contentVersion: "2", createdAt: "2026-11-18T00:00:00Z", available: true }], includesWholeConversation: false });
const results = [];
function test(name, action) { try { action(); results.push({ name, status: "passed" }); } catch (error) { results.push({ name, status: "failed", error: error.message }); } }
const reject = action => assert.throws(action);
test("empty and combined report filters use only the two UUID query parameters", () => {
  assert.equal(protocol.reportListPath("", ""), "/admin/reports");
  assert.equal(protocol.reportListPath(" " + actorId + " ", reportId), "/admin/reports?reporterId=" + actorId + "&targetUserId=" + reportId);
});
test("malformed filters cannot add query fields or invoke another path", () => {
  for (const input of ["name", actorId + "&admin=true", "../reports", actorId + "," + reportId]) reject(() => protocol.reportListPath(input, ""));
});
test("evidence route binds both the exact object and report", () => {
  assert.equal(protocol.reportMediaPath(evidenceURL, reportId, objectId, "evidence"), evidenceURL.slice(4));
  reject(() => protocol.reportMediaPath(evidenceURL, actorId, objectId, "evidence"));
  reject(() => protocol.reportMediaPath(evidenceURL, reportId, actorId, "evidence"));
});
test("duplicate query, extra parameters and fragments are rejected", () => {
  for (const url of [evidenceURL + "&reportId=" + reportId, evidenceURL + "&download=1", evidenceURL + "#private"]) reject(() => protocol.reportMediaPath(url, reportId, objectId, "evidence"));
});
test("foreign, absolute and traversal media addresses never receive current auth", () => {
  for (const url of ["https://foreign.test" + evidenceURL, "//foreign.test" + evidenceURL, "https://jobsim.invalid" + evidenceURL, "/api/admin/reports/../reports/evidence/" + objectId + "?reportId=" + reportId]) reject(() => protocol.reportMediaPath(url, reportId, objectId, "evidence"));
});
test("selected attachments bind the exact report and message without extra query", () => {
  assert.equal(protocol.reportMediaPath(messageURL, reportId, messageId, "message"), messageURL.slice(4));
  for (const url of [messageURL + "?download=true", messageURL.replace(messageId, actorId), messageURL.replace(reportId, actorId)]) reject(() => protocol.reportMediaPath(url, reportId, messageId, "message"));
});
test("detail refuses a different report or any whole-conversation declaration", () => {
  const wrong = detail(); wrong.includesWholeConversation = true; reject(() => protocol.checkedReportDetail(wrong, reportId));
  reject(() => protocol.checkedReportDetail(detail(), actorId));
});
test("unavailable selected messages strip stale body, metadata and attachment URL", () => {
  const value = detail(); value.selectedMessages = [{ id: messageId, available: false, text: "must disappear", attachmentURL: messageURL }];
  const result = protocol.checkedReportDetail(value, reportId).selectedMessages[0];
  assert.equal(result.text, ""); assert.equal(result.attachmentURL, undefined); assert.equal(result.senderId, "");
});
test("unavailable evidence strips even an old valid media URL", () => {
  const value = detail(); value.evidence[0].available = false;
  assert.equal(protocol.checkedReportDetail(value, reportId).evidence[0].url, "");
});
test("negative sizes, forged hashes and duplicate objects are rejected", () => {
  for (const mutate of [v => v.evidence[0].size = -1, v => v.evidence[0].sha256 = "wrong", v => v.evidence.push(v.evidence[0])]) { const value = detail(); mutate(value); reject(() => protocol.checkedReportDetail(value, reportId)); }
});
test("selected content versions retain canonical decimal string semantics", () => {
  for (const version of [2, "02", "1.2", "-1"]) { const value = detail(); value.selectedMessages[0].contentVersion = version; reject(() => protocol.checkedReportDetail(value, reportId)); }
});
test("PNG JPEG and MP4 signatures can be previewed with matching declared kinds", () => {
  assert.equal(protocol.reportPreviewMime(Uint8Array.from([137,80,78,71,13,10,26,10]), "image"), "image/png");
  assert.equal(protocol.reportPreviewMime(Uint8Array.from([255,216,255]), "image/jpeg"), "image/jpeg");
  assert.equal(protocol.reportPreviewMime(Uint8Array.from([0,0,0,20,102,116,121,112,105,115,111,109]), "video"), "video/mp4");
});
test("HTML SVG unknown bytes and declared media mismatch are never rendered", () => {
  for (const text of ["<svg onload=alert(1)>", "<html>private</html>", "not media"]) reject(() => protocol.reportPreviewMime(new TextEncoder().encode(text), "image"));
  reject(() => protocol.reportPreviewMime(Uint8Array.from([137,80,78,71,13,10,26,10]), "video/mp4"));
});
test("image and video bounds preserve the valid 32-to-64MiB video range", () => {
  assert.equal(protocol.reportMediaLimit("image/png"), 20 * 1024 * 1024);
  assert.equal(protocol.reportMediaLimit("image"), 20 * 1024 * 1024);
  assert.equal(protocol.reportMediaLimit("video/mp4"), 64 * 1024 * 1024);
  assert.equal(protocol.reportMediaLimit("video"), 64 * 1024 * 1024);
  assert.ok(protocol.reportMediaLimit("video/mp4") >= 40 * 1024 * 1024);
  assert.equal(protocol.reportMediaLimit("image/svg+xml"), 0);
});
test("all ten admin-report locales contain the same eleven nonempty keys", () => {
  assert.equal(Object.keys(labels).length, 10); const keys = Object.keys(labels.en).sort(); assert.equal(keys.length, 11);
  for (const values of Object.values(labels)) { assert.deepEqual(Object.keys(values).sort(), keys); assert.ok(Object.values(values).every(v => v.trim())); }
});
test("the existing MFA apply and unknown-result retry flow remains byte-identical", () => {
  const before = fs.readFileSync(path.join(root, "项目相关报告/测试/前端/Web/20261118-管理员举报证据/before/AdminPanel.tsx"), "utf8"), current = fs.readFileSync(path.join(web, "Src/AdminPanel.tsx"), "utf8");
  const apply = source => source.slice(source.indexOf("  const apply = async"), source.indexOf("  const empty =")); assert.equal(apply(current), apply(before));
});
const summary = { scope: "Node/actual production protocol functions and localization; no browser or live admin API", passed: results.filter(r => r.status === "passed").length, failed: results.filter(r => r.status === "failed").length, cases: results };
console.log(JSON.stringify(summary, null, 2)); if (summary.failed) process.exitCode = 1;
