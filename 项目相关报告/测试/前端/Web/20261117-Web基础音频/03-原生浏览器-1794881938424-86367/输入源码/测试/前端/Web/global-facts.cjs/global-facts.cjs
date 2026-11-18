#!/usr/bin/env node
"use strict";

// Local IndexedDB module regression. Every /api request is aborted; no server fixture is used.
const fs = require("node:fs"), path = require("node:path"), os = require("node:os"), crypto = require("node:crypto");
const root = path.resolve(__dirname, "../../../..");
const { chromium } = require(process.env.JOBSIM_PLAYWRIGHT_MODULE || process.env.PLAYWRIGHT_MODULE || "playwright");
const origin = process.env.JOBSIM_WEB_URL || process.env.JOBSIM_WEB_ORIGIN || "http://127.0.0.1:5189";
if (!["localhost", "127.0.0.1"].includes(new URL(origin).hostname)) throw new Error("Use an existing local Vite test server");
const report = process.env.JOBSIM_GLOBAL_REPORT || path.join(root, "项目相关报告/测试/前端/Web/执行记录", `global-facts-${Date.now()}-${process.pid}`);
if (fs.existsSync(report) && fs.readdirSync(report).length) throw new Error("Refusing to overwrite an existing report");
fs.mkdirSync(report, { recursive: true });
const inputs = ["GlobalMessages.ts", "StorageDatabase.ts", "storage.ts", "SyncStore.ts", "types.ts", "forward.ts", "SyncProtocol.ts"].map(name => `代码/前端/Web/Src/${name}`);
inputs.push(path.relative(root, __filename));
const inputSHA256 = Object.fromEntries(inputs.map(name => {
  const bytes = fs.readFileSync(path.join(root, name)), destination = path.join(report, "输入源码", name);
  fs.mkdirSync(path.dirname(destination), { recursive: true }); fs.writeFileSync(destination, bytes, { flag: "wx" });
  return [name, crypto.createHash("sha256").update(bytes).digest("hex")];
}));

(async () => {
  const results = [];
  const browser = await chromium.launch({ executablePath: process.env.JOBSIM_CHROME || "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", headless: true });
  const watchdog = setTimeout(() => browser.close().catch(() => {}), 60000);
  let apiAttempts = 0;
  try {
    const context = await browser.newContext();
    await context.route("**/api/**", route => { apiAttempts++; return route.abort(); });
    await context.route(origin + "/global-facts-test", route => route.fulfill({ contentType: "text/html", body: "<!doctype html><meta charset=utf-8><title>Local global facts test</title>" }));
    const page = await context.newPage();
    page.setDefaultTimeout(10000);
    await page.goto(origin + "/global-facts-test", { timeout: 15000 });
    await page.evaluate(async () => {
      const G = await import("/Src/GlobalMessages.ts"), S = await import("/Src/storage.ts"), Y = await import("/Src/SyncStore.ts"), T = await import("/Src/types.ts");
      const db = await S.storageDatabase;
      const write = entries => new Promise((resolve, reject) => {
        const tx = db.transaction("spaces", "readwrite", { durability: "strict" });
        for (const [key, value] of entries) tx.objectStore("spaces").put(value, key);
        tx.oncomplete = resolve; tx.onerror = () => reject(tx.error); tx.onabort = () => reject(tx.error);
      });
      const read = key => new Promise((resolve, reject) => {
        const tx = db.transaction("spaces"), request = tx.objectStore("spaces").get(key);
        tx.oncomplete = () => resolve(request.result); tx.onerror = () => reject(tx.error);
      });
      const check = (condition, label) => { if (!condition) throw new Error(label); };
      const create = async (modern = true) => {
        const f = { deployment: crypto.randomUUID(), user: crypto.randomUUID(), device: crypto.randomUUID(), conversation: crypto.randomUUID(), id: crypto.randomUUID() };
        f.scope = `${location.origin}|${f.deployment}|${f.user}|personal`;
        f.namespace = f.scope + (modern ? "|1|" : "|") + f.device;
        f.message = { id: f.id, conversationId: f.conversation, senderId: f.user, clientMsgId: "0123456789abcdef0123456789abcdef", type: "text", text: "private source", version: "1", contentVersion: "1", sequence: "1", createdAt: "2026-11-16T01:00:00Z", updatedAt: "2026-11-16T01:00:00Z", deleted: false, reactions: [], pinned: false, favorite: false, attachmentURL: "/controlled-sidecar", attachmentName: "private-name" };
        f.pending = { ...f.message, id: crypto.randomUUID(), text: "ORIGINAL_UNKNOWN_BODY", pending: true, sendAttempted: true, sendAdmission: "ORIGINAL_ADMISSION" };
        f.space = { ...T.emptySpace(), messages: { [f.conversation]: [f.message] }, drafts: { [f.conversation]: "ORIGINAL_DRAFT" }, unread: { [f.conversation]: [f.id] }, mentionUnread: { [f.conversation]: [f.id] }, outbox: [f.pending] };
        await write([[f.namespace, f.space], ["outbox|" + f.namespace, [f.pending]], ...(modern ? [["auth-active|" + f.scope, f.namespace]] : [])]);
        return f;
      };
      const state = async f => {
        const identity = { scopeId: f.deployment, principalId: f.user, sourceContextId: "personal", restoreEpoch: "1", deviceId: f.device };
        const value = { schema: "1", identity, storeGeneration: crypto.randomUUID(), registrationRequestId: crypto.randomUUID(), guardGeneration: crypto.randomUUID(), streamId: crypto.randomUUID(), cursor: "original-cursor", position: "0", classificationWatermark: "0", permissionGeneration: "a".repeat(64), projections: {} };
        await write([["sync-state|" + f.namespace, value]]); return value;
      };
      window.gf = { G, S, Y, T, write, read, create, state, check };
    });
    const test = async (name, body) => {
      const started = Date.now();
      try { await page.evaluate(body); results.push({ name, status: "passed", milliseconds: Date.now() - started }); }
      catch (error) { results.push({ name, status: "failed", error: String(error), milliseconds: Date.now() - started }); }
    };
    await test("terminal facts clear current old-device epoch archive and retained without touching other scopes or original Outbox", async () => {
      const { G, write, read, create, check } = window.gf, f = await create();
      const older = f.scope + "|2|" + crypto.randomUUID(), legacy = f.scope + "|" + crypto.randomUUID();
      const otherProfile = f.namespace.replace(location.origin, "https://other-profile.example"), otherUser = f.namespace.replace(f.user, crypto.randomUUID());
      const keys = [older, legacy, older + "|archive|1", "sync-retained|" + older];
      const forward = { ...f.message, id: crypto.randomUUID(), forwarded: true, forwardAdmission: "TEST_SOURCE_PROOF", text: "controlled forward" };
      await write([...keys.map(key => [key, { ...f.space, messages: { [f.conversation]: [f.message, forward] } }]), [otherProfile, f.space], [otherUser, f.space], ["unknown-old-hash", f.space]]);
      await G.observeVerifiedGlobals(f.namespace, [f.id], () => true);
      for (const key of [f.namespace, ...keys]) {
        const value = await read(key), deleted = value.messages[f.conversation].find(row => row.id === f.id);
        check(deleted.deleted && deleted.text === "" && !deleted.attachmentURL && !deleted.attachmentName, "global retained body/sidecar survived");
        check(value.drafts[f.conversation] === "ORIGINAL_DRAFT", "draft changed");
        check(JSON.stringify(value.outbox) === JSON.stringify([f.pending]), "nested original Outbox changed");
        const heldForward = value.messages[f.conversation].find(row => row.id === forward.id);
        if (heldForward) check(heldForward.text === "" && !heldForward.forwardAdmission, "forward body persisted");
      }
      for (const key of [otherProfile, otherUser, "unknown-old-hash"]) check((await read(key)).messages[f.conversation][0].text === "private source", "unverified scope was changed");
      check(JSON.stringify(await read("outbox|" + f.namespace)) === JSON.stringify([f.pending]), "queue acknowledged or payload changed");
    });
    await test("readonly complete ledger does not create durable sync responsibility and RAM mask only changes specified IDs", async () => {
      const { G, Y, create, check } = window.gf, f = await create(false);
      check((await G.loadVerifiedGlobals(f.namespace)).length === 0, "read invented facts");
      await G.observeVerifiedGlobals(f.namespace, [f.id, f.id], () => true);
      check(JSON.stringify(await G.loadVerifiedGlobals(f.namespace)) === JSON.stringify([f.id]), "ledger incomplete or duplicated");
      check(!await Y.hasDurableSyncResponsibility(f.namespace), "global-only legacy namespace became sync-required");
      const forwarded = { ...f.message, id: crypto.randomUUID(), forwarded: true, text: "fresh RAM forward" };
      const masked = G.applyKnownGlobals({ ...f.space, messages: { [f.conversation]: [f.message, forwarded, f.pending] } }, [f.id]);
      check(masked.messages[f.conversation][0].deleted, "RAM global not hidden");
      check(masked.messages[f.conversation][1].text === "fresh RAM forward", "unrelated fresh RAM forward was masked");
      check(masked.messages[f.conversation][2].text === f.pending.text, "pending original was masked");
    });
    await test("high-version stale save freeze and enqueue cannot restore a known body and accepted CID does not hide pending", async () => {
      const { G, S, read, create, check } = window.gf, f = await create();
      await G.observeVerifiedGlobals(f.namespace, [f.id], () => true);
      const stale = { ...f.space, messages: { [f.conversation]: [{ ...f.message, version: "999" }] } };
      await S.saveView(f.namespace, stale);
      await S.freezeConversation(f.namespace, crypto.randomUUID(), stale);
      await S.queueMessage(f.namespace, stale, f.pending);
      const stored = await S.loadSpace(f.namespace);
      check(stored.messages[f.conversation].find(row => !row.pending).deleted, "stale available revived");
      check(stored.messages[f.conversation].some(row => row.pending && row.text === f.pending.text), "accepted CID hid original pending bubble");
      check(JSON.stringify(await read("outbox|" + f.namespace)) === JSON.stringify([f.pending]), "enqueue changed original intent");
    });
    await test("late lifecycle active mismatch and malformed object IDs produce no facts", async () => {
      const { G, write, create, check } = window.gf, f = await create();
      let calls = 0, rejected = false;
      try { await G.observeVerifiedGlobals(f.namespace, [f.id], () => ++calls === 1); } catch { rejected = true; }
      check(rejected && (await G.loadVerifiedGlobals(f.namespace)).length === 0, "late context committed global");
      await write([["auth-active|" + f.scope, f.scope + "|1|" + crypto.randomUUID()]]);
      rejected = false;
      try { await G.observeVerifiedGlobals(f.namespace, [f.id], () => true); } catch { rejected = true; }
      check(rejected && (await G.loadVerifiedGlobals(f.namespace)).length === 0, "inactive device committed global");
      rejected = false;
      try { await G.observeVerifiedGlobals(f.namespace, ["0123456789abcdef0123456789abcdef"], () => true); } catch { rejected = true; }
      check(rejected, "opaque client ID was accepted as a server object ID");
    });
    await test("external verified fact advances existing guard and higher delta cannot resurrect or ACK original", async () => {
      const { G, Y, read, create, state, check } = window.gf, f = await create(), original = await state(f);
      await G.observeVerifiedGlobals(f.namespace, [f.id], () => true);
      const current = await read("sync-state|" + f.namespace);
      check(original.guardGeneration !== current.guardGeneration, "persistent guard did not advance");
      const projection = { ref: { owner: "message", kind: "message", id: f.id, revision: "999", revisionEpoch: "1", state: "active", conversationId: f.conversation }, state: "available", currentRevision: "999", currentRevisionEpoch: "1", revisionMatch: true, object: { ...f.message, version: "999" } };
      const result = await Y.commitSyncProjection({ namespace: f.namespace, identity: current.identity, binding: { ...current.identity, storeGeneration: current.storeGeneration, streamId: current.streamId }, expectedCursor: current.cursor, expectedGuardGeneration: current.guardGeneration, cursor: "next-cursor", position: "1", permissionGeneration: current.permissionGeneration, classificationWatermark: "0", projections: [projection], materialize: (latest, rows) => ({ ...latest, messages: { ...latest.messages, [f.conversation]: [...latest.messages[f.conversation], ...rows.map(row => row.object)] } }), isCurrent: () => true });
      check(result.space.messages[f.conversation].every(row => row.pending || row.deleted), "higher sync value revived global");
      check(JSON.stringify(await read("outbox|" + f.namespace)) === JSON.stringify([f.pending]), "same CID sync implicitly ACKed original");
    });
    await test("validated global sync projection shares the ledger atomically with cursor and future legacy writes", async () => {
      const { G, S, Y, read, write, create, state, check } = window.gf, f = await create(), current = await state(f);
      const older = f.scope + "|" + crypto.randomUUID(); await write([[older, f.space]]);
      const projection = { ref: { owner: "message", kind: "message", id: f.id, revision: "2", revisionEpoch: "1", state: "active", conversationId: f.conversation }, state: "global", currentRevision: "2", currentRevisionEpoch: "1", revisionMatch: true, object: { id: f.id } };
      await Y.commitSyncProjection({ namespace: f.namespace, identity: current.identity, binding: { ...current.identity, storeGeneration: current.storeGeneration, streamId: current.streamId }, expectedCursor: current.cursor, expectedGuardGeneration: current.guardGeneration, cursor: "global-cursor", position: "1", permissionGeneration: current.permissionGeneration, classificationWatermark: "0", projections: [projection], materialize: latest => ({ ...latest, messages: { [f.conversation]: [{ ...f.message, text: "", deleted: true }] } }), isCurrent: () => true });
      check((await read("sync-state|" + f.namespace)).cursor === "global-cursor", "global cursor not committed");
      check((await G.loadVerifiedGlobals(f.namespace)).includes(f.id), "sync global missing ledger");
      await S.saveView(older, f.space);
      check((await read(older)).messages[f.conversation][0].deleted, "legacy stale write bypassed global ledger");
    });
  } finally {
    clearTimeout(watchdog); await browser.close();
    const value = { scope: "Real Chrome IndexedDB and injected local projections; all API routes aborted", apiAttempts, inputSHA256, results, passed: results.filter(row => row.status === "passed").length, failed: results.filter(row => row.status === "failed").length };
    fs.writeFileSync(path.join(report, "结果.json"), JSON.stringify(value, null, 2) + "\n", { flag: "wx" });
    console.log(JSON.stringify({ report, passed: value.passed, failed: value.failed, apiAttempts }));
    if (value.failed || value.passed !== 6) process.exitCode = 1;
  }
})().catch(error => { console.error(String(error)); process.exitCode = 1; });
