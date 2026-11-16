// Created by Jobs on 2026年11月16日，星期一.
const fs = require("node:fs"), path = require("node:path"), crypto = require("node:crypto");
const { chromium } = require(process.env.JOBSIM_PLAYWRIGHT_MODULE || "playwright");
const root = path.resolve(__dirname, "../../../..");
const out = process.env.JOBSIM_SYNC_REPORT || path.join(root, "项目相关报告/测试/前端/Web/执行记录", "sync-store-" + Date.now() + "-" + process.pid);
if (fs.existsSync(out)) throw Error("Choose a new report directory");
fs.mkdirSync(out, { recursive: true });
const checks = [], started = new Date().toISOString();
const files = ["SyncStore.ts", "SyncEngine.ts", "SyncProtocol.ts", "SyncView.ts", "storage.ts", "types.ts", "forward.ts"].map(name => "代码/前端/Web/Src/" + name);
files.push("测试/前端/Web/sync-store.cjs/sync-store.cjs");
const hashes = () => Object.fromEntries(files.map(name => [name, crypto.createHash("sha256").update(fs.readFileSync(path.join(root, name))).digest("hex")]));
const sourceSHA256 = hashes();
const assert = (value, label) => { if (!value) throw Error(label); };

async function prepare(page) {
  await page.route("**/api/**", route => route.abort());
  await page.goto(process.env.JOBSIM_WEB_URL || "http://127.0.0.1:5189", { waitUntil: "domcontentloaded" });
  await page.evaluate(async () => {
    const store = await import("/Src/SyncStore.ts"), engine = await import("/Src/SyncEngine.ts");
    const storage = await import("/Src/storage.ts"), view = await import("/Src/SyncView.ts"), types = await import("/Src/types.ts");
    const db = await storage.storageDatabase;
    const hash = async value => [...new Uint8Array(await crypto.subtle.digest("SHA-256", new TextEncoder().encode(value)))].map(v => v.toString(16).padStart(2, "0")).join("");
    const envelope = value => btoa(unescape(encodeURIComponent(JSON.stringify(value)))).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
    const read = key => new Promise((resolve, reject) => {
      const tx = db.transaction("spaces"), q = tx.objectStore("spaces").get(key);
      tx.oncomplete = () => resolve(q.result); tx.onerror = () => reject(tx.error);
    });
    const write = entries => new Promise((resolve, reject) => {
      const tx = db.transaction("spaces", "readwrite", { durability: "strict" }), target = tx.objectStore("spaces");
      for (const [key, value] of entries) target.put(value, key);
      tx.oncomplete = () => resolve(); tx.onerror = () => reject(tx.error); tx.onabort = () => reject(tx.error);
    });
    const create = async (data, seed = true) => {
      const f = Object.assign({ scopeId: crypto.randomUUID(), user: crypto.randomUUID(), device: crypto.randomUUID(), conversation: crypto.randomUUID(), message: crypto.randomUUID(), client: crypto.randomUUID(), stream: crypto.randomUUID(), snapshot: crypto.randomUUID(), origin: crypto.randomUUID() }, data || {});
      f.current = true; f.capability = true; f.log = []; f.restricted = []; f.delta = null;
      f.namespace = location.origin + "|" + f.scopeId + "|" + f.user + "|personal|1|" + f.device;
      f.binding = { scopeId: f.scopeId, principalId: f.user, sourceContextId: "personal", deviceId: f.device, restoreEpoch: "1" };
      f.permission = await hash("fixture-permission");
      f.ref = (kind, id, revision = "1") => ({ owner: kind === "conversation" ? "conversation" : "message", kind, id, revision, revisionEpoch: "1", ...(kind === "message" ? { subjectUserId: f.user } : {}), conversationId: f.conversation, state: "active" });
      f.conversationRef = f.ref("conversation", f.conversation); f.messageRef = f.ref("message", f.message);
      f.conversationObject = { id: f.conversation, kind: "group", title: "Scoped fixture", ownerId: f.user, memberIds: [f.user], updatedAt: "2026-11-15T00:00:00Z", cursor: "0" };
      f.messageObject = { id: f.message, conversationId: f.conversation, senderId: f.user, clientMsgId: f.client, type: "text", text: "SCOPED_OLD_BODY", version: "1", contentVersion: "1", sequence: "1", deleted: false, reactions: [], pinned: false, favorite: false, createdAt: "2026-11-15T00:00:00Z", updatedAt: "2026-11-15T00:00:00Z" };
      f.projection = async (ref, object, state = "available", currentRevision = ref.revision) => ({ ref, state, currentRevision, currentRevisionEpoch: "1", revisionMatch: currentRevision === ref.revision, bodyEnvelope: envelope(object), bodyDigest: await hash(JSON.stringify(object)), object });
      f.pages = [];
      for (const [ref, object] of [[f.conversationRef, f.conversationObject], [f.messageRef, f.messageObject]]) {
        const refs = [ref], page = f.pages.length, digest = await hash(JSON.stringify(refs));
        f.pages.push({ snapshotId: f.snapshot, page, refs, referenceEnvelope: envelope(refs), pageDigest: digest, items: [await f.projection(ref, object)], permissionGeneration: f.permission, restoreEpoch: "1", digestScope: "immutable_references_only", installable: true, requiresFinalValidation: true, requiresAtomicInstall: true, qualifiedAck: false });
      }
      f.manifest = { ...f.binding, storeGeneration: "", streamId: f.stream, snapshotId: f.snapshot, schemaVersion: "1", boundary: "0", boundaryToken: "fixture-boundary", ownerWatermarks: { auth: "0", directory: "0", conversation: "1", message: "1" }, permissionGeneration: f.permission, classificationWatermark: "1", coverage: {}, pageManifest: f.pages.map(p => ({ page: p.page, count: 1, digest: p.pageDigest })), expiresAt: new Date(Date.now() + 600000).toISOString() };
      f.event = async projection => {
        const immutable = { position: "1", originEventId: f.origin, ref: projection.ref };
        return { kind: "reference", ...immutable, eventEnvelope: envelope(immutable), eventDigest: await hash(JSON.stringify(immutable)), projection };
      };
      f.request = async (url, options = {}) => {
        const body = options.body ? JSON.parse(options.body) : undefined;
        f.log.push({ url, body });
        if (url === "/bootstrap") return { deploymentId: f.scopeId, restoreEpoch: "1", features: { authoritative_sync: f.capability } };
        if (url === "/sync/device") { f.manifest.storeGeneration = body.storeGeneration; return { ...f.binding, storeGeneration: body.storeGeneration, streamId: f.stream, qualification: "transport_only", qualifiedAck: false }; }
        if (url === "/sync/context") {
          if (f.pauseContext) await new Promise(resolve => { f.release = resolve; });
          return { ...f.binding, storeGeneration: f.manifest.storeGeneration, streamId: f.stream, head: f.delta ? "1" : "0", permissionGeneration: f.permission, qualification: "transport_only", qualifiedAck: false, afterReceiptEnabled: false };
        }
        if (url === "/sync/snapshots") {
          if (f.loseSnapshot) { f.loseSnapshot = false; throw Object.assign(Error("Scoped lost result"), { code: "SCOPED_NETWORK_UNKNOWN" }); }
          return f.manifest;
        }
        const match = url.match(/\/pages\/(\d+)$/);
        if (match) { if (f.failPage === Number(match[1])) throw Object.assign(Error("Scoped page fault"), { code: "SCOPED_PAGE_UNKNOWN" }); return f.pages[Number(match[1])]; }
        if (url.endsWith("/validate")) {
          if (f.removeStageBeforeCommit) {
            const key = "sync-stage|" + f.namespace + "|" + f.snapshot, stage = await read(key); delete stage.pages["1"]; await write([[key, stage]]);
          }
          return { canInstall: true, boundaryToken: f.manifest.boundaryToken, restoreEpoch: "1", permissionGeneration: f.permission, classificationWatermark: "1", requiresLocalAtomicInstall: true, qualifiedAck: false };
        }
        if (url.startsWith("/sync/changes?")) {
          if (f.restrictBeforeCommit) await store.restrictSyncProjection(f.namespace, f.binding, "SCOPED_NEW_RESTRICTION", () => true);
          return { items: f.delta ? [f.delta] : [], cursor: f.delta ? "fixture-cursor-1" : "fixture-cursor-0", position: f.delta ? "1" : "0", head: f.delta ? "1" : "0", hasMore: false, restoreEpoch: "1", permissionGeneration: f.permission, snapshotRequired: false, qualifiedAck: false };
        }
        throw Error("Unexpected simulated route");
      };
      f.run = () => engine.runDurableSync({ namespace: f.namespace, binding: f.binding, request: f.request, isCurrent: () => f.current, materialize: view.materializeSyncSpace, onRestriction: code => f.restricted.push(code) });
      if (seed) await write([["auth-active|" + f.namespace.split("|").slice(0, 4).join("|"), f.namespace], [f.namespace, { ...types.emptySpace(), drafts: { [f.conversation]: "ORIGINAL_DRAFT" } }]]);
      return f;
    };
    const data = f => Object.fromEntries(["scopeId", "user", "device", "conversation", "message", "client", "stream", "snapshot", "origin"].map(k => [k, f[k]]));
    window.syncTest = { create, data, read, write, store, engine, storage, view, types, db };
  });
}

(async () => {
  const browser = await chromium.launch({ headless: true, executablePath: process.env.BROWSER_EXECUTABLE || "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" });
  const context = await browser.newContext(), page = await context.newPage();
  try {
    await prepare(page);
    async function test(name, callback) {
      await callback(); checks.push({ name, status: "passed" }); console.log("PASS " + name);
    }
    await test("single flight and snapshot/cursor atomic install", async () => assert(await page.evaluate(async () => {
      const t = window.syncTest, f = await t.create();
      const all = await Promise.all(Array.from({ length: 6 }, () => f.run()));
      const state = await t.read("sync-state|" + f.namespace), space = await t.read(f.namespace);
      return f.log.filter(x => x.url === "/sync/snapshots").length === 1 && all.every(x => x.position === "0") && state.cursor === "fixture-cursor-0" && space.messages[f.conversation][0].text === "SCOPED_OLD_BODY" && space.drafts[f.conversation] === "ORIGINAL_DRAFT";
    }), "atomic initial install"));
    await test("missing persisted page refuses install", async () => assert(await page.evaluate(async () => {
      const t = window.syncTest, f = await t.create(); f.removeStageBeforeCommit = true;
      let code; try { await f.run(); } catch (e) { code = e.code; }
      const state = await t.read("sync-state|" + f.namespace);
      return code === "SYNC_STAGE_INCOMPLETE" && !state.cursor && !!state.pendingManifest;
    }), "missing stage committed"));
    await test("late context cannot clear a new device namespace", async () => {
      await page.evaluate(async () => { const f = await window.syncTest.create(); window.lateFixture = f; f.pauseContext = true; window.lateRun = f.run().then(() => "unexpected", e => e.code); });
      await page.waitForFunction(() => !!window.lateFixture.release);
      assert(await page.evaluate(async () => {
        const t = window.syncTest, f = window.lateFixture, other = f.namespace.replace(f.device, crypto.randomUUID());
        await t.write([[other, { ...t.types.emptySpace(), drafts: { marker: "NEW_DEVICE_SAFE" } }]]);
        f.current = false; f.release(); const code = await window.lateRun, saved = await t.read(other);
        return code === "SYNC_CONTEXT_CHANGED" && saved.drafts.marker === "NEW_DEVICE_SAFE";
      }), "late old context changed new store");
    });
    await test("old saveView cannot overwrite newer sync state", async () => assert(await page.evaluate(async () => {
      const t = window.syncTest, f = await t.create(), old = await t.read(f.namespace); await f.run();
      old.drafts[f.conversation] = "LATE_DRAFT"; old.messages = {}; await t.storage.saveView(f.namespace, old);
      const space = await t.read(f.namespace), state = await t.read("sync-state|" + f.namespace);
      return space.drafts[f.conversation] === "LATE_DRAFT" && space.messages[f.conversation][0].text === "SCOPED_OLD_BODY" && state.cursor === "fixture-cursor-0";
    }), "old view erased sync"));
    await test("unknown delta retains cursor and isolated history", async () => assert(await page.evaluate(async () => {
      const t = window.syncTest, f = await t.create(); await f.run();
      f.delta = await f.event({ ref: f.messageRef, state: "unknown", revisionMatch: false });
      let code; try { await f.run(); } catch (e) { code = e.code; }
      const state = await t.read("sync-state|" + f.namespace), held = await t.read("sync-retained|" + f.namespace), space = await t.read(f.namespace);
      return code === "SYNC_PROJECTION_UNKNOWN" && state.cursor === "fixture-cursor-0" && held.messages[f.conversation][0].text === "SCOPED_OLD_BODY" && held.messages[f.conversation][0].deleted === false && !space.messages[f.conversation];
    }), "unknown advanced cursor or destroyed history"));
    await test("global clears duplicate holders and masks unrelated forward bodies", async () => assert(await page.evaluate(async () => {
      const t = window.syncTest, f = await t.create(); await f.run(); const space = await t.read(f.namespace);
      const forward = { ...f.messageObject, id: crypto.randomUUID(), forwarded: true, text: "FORWARD_SECRET", attachmentURL: "/api/messages/fixture/attachment" };
      const duplicate = { ...space, messages: { [f.conversation]: [f.messageObject, forward] } }, other = f.namespace.replace(f.device, crypto.randomUUID());
      await t.write([[other, duplicate], [f.namespace + "|archive|1", duplicate], ["sync-retained|" + other, duplicate], ["sync-retained|" + f.namespace, duplicate]]);
      const ref = { ...f.messageRef, revision: "2" }; f.delta = await f.event(await f.projection(ref, { id: f.message, conversationId: f.conversation, deleted: true, version: "2", sequence: "1" }, "global")); await f.run();
      for (const key of [other, f.namespace + "|archive|1", "sync-retained|" + other, "sync-retained|" + f.namespace]) {
        const value = await t.read(key), raw = JSON.stringify(value); if (raw.includes("SCOPED_OLD_BODY") || raw.includes("FORWARD_SECRET")) return false;
        const projected = Object.values(value.messages).flat().find(row => row.id === forward.id); if (projected && projected.deleted) return false;
      }
      return true;
    }), "global holder retained body"));
    await test("unknown snapshot POST keeps original request through cold reload", async () => {
      const data = await page.evaluate(async () => {
        const t = window.syncTest, f = await t.create(); f.loseSnapshot = true; try { await f.run(); } catch {}
        const state = await t.read("sync-state|" + f.namespace); return { ...t.data(f), requestId: state.snapshotIntent.requestId };
      });
      await prepare(page);
      assert(await page.evaluate(async data => {
        const t = window.syncTest, f = await t.create(data, false); await f.run();
        return f.log.find(x => x.url === "/sync/snapshots").body.requestId === data.requestId && f.log.filter(x => /\/pages\//.test(x.url)).length === 2;
      }, data), "cold retry changed original ID");
    });
    await test("partial staging survives and cold resume re-fetches every page", async () => {
      const data = await page.evaluate(async () => {
        const t = window.syncTest, f = await t.create(); f.failPage = 1; try { await f.run(); } catch {}
        const state = await t.read("sync-state|" + f.namespace), stage = await t.read("sync-stage|" + f.namespace + "|" + f.snapshot);
        if (Object.keys(stage.pages).length !== 1) throw Error("partial page lost"); return { ...t.data(f), requestId: state.snapshotIntent.requestId };
      });
      await prepare(page);
      assert(await page.evaluate(async data => {
        const t = window.syncTest, f = await t.create(data, false); await f.run();
        return f.log.filter(x => /\/pages\//.test(x.url)).length === 2 && f.log.find(x => x.url === "/sync/snapshots").body.requestId === data.requestId;
      }, data), "cold stage promoted without re-fetch");
    });
    await test("capability absent creates no durable sync responsibility", async () => assert(await page.evaluate(async () => {
      const t = window.syncTest, f = await t.create(); f.capability = false; let code; try { await f.run(); } catch (e) { code = e.code; }
      return code === "SYNC_CAPABILITY_REQUIRED" && !(await t.store.hasDurableSyncResponsibility(f.namespace)) && !f.log.some(x => x.url === "/sync/device");
    }), "false flag created responsibility"));
    await test("native IDB abort rolls back view and cursor together", async () => assert(await page.evaluate(async () => {
      const t = window.syncTest, f = await t.create(); await f.run();
      const before = await t.read(f.namespace), state = await t.read("sync-state|" + f.namespace); let rejected = false;
      try {
        await t.store.commitSyncProjection({ namespace: f.namespace, identity: f.binding, binding: { ...f.binding, streamId: f.stream, storeGeneration: state.storeGeneration }, expectedGuardGeneration: state.guardGeneration, expectedCursor: state.cursor, cursor: "must-not-commit", position: "1", permissionGeneration: f.permission, classificationWatermark: "1", projections: [], isCurrent: () => true, materialize: latest => ({ ...latest, nonCloneable: () => true }) });
      } catch { rejected = true; }
      const after = await t.read(f.namespace), metadata = await t.read("sync-state|" + f.namespace);
      return rejected && before.syncRevision === after.syncRevision && metadata.cursor === state.cursor && metadata.position === state.position;
    }), "native IDB partial commit"));
    await test("new local restriction fences late validated snapshot", async () => assert(await page.evaluate(async () => {
      const t = window.syncTest, f = await t.create(); f.restrictBeforeCommit = true;
      let code; try { await f.run(); } catch (e) { code = e.code; }
      const state = await t.read("sync-state|" + f.namespace), space = await t.read(f.namespace);
      return code === "SYNC_LOCAL_RESTRICTION_CHANGED" && !state.cursor && !space.messages[f.conversation];
    }), "late snapshot cleared new restriction"));
    await test("two tabs serialize through real WebLocks and one registration identity", async () => {
      const data = await page.evaluate(async () => { const t = window.syncTest, f = await t.create(); window.crossFixture = f; return t.data(f); });
      const other = await context.newPage(); await prepare(other);
      await other.evaluate(async data => { window.crossFixture = await window.syncTest.create(data, false); }, data);
      await Promise.all([page.evaluate(() => window.crossFixture.run().then(() => true)), other.evaluate(() => window.crossFixture.run().then(() => true))]);
      const logs = [...await page.evaluate(() => window.crossFixture.log), ...await other.evaluate(() => window.crossFixture.log)];
      const registration = logs.filter(x => x.url === "/sync/device");
      assert(logs.filter(x => x.url === "/sync/snapshots").length === 1 && registration.length === 2 && registration[0].body.requestId === registration[1].body.requestId && registration[0].body.storeGeneration === registration[1].body.storeGeneration, "cross-tab lock or identity mismatch");
      await other.close();
    });
    assert(JSON.stringify(sourceSHA256) === JSON.stringify(hashes()), "Source changed during browser execution");
  } finally {
    await browser.close();
    fs.writeFileSync(path.join(out, "结果.json"), JSON.stringify({ at: started, scope: "Real Chrome IndexedDB/WebLocks with explicit simulated API; /api requests blocked; not backend HTTPS or qualified ACK", status: checks.length === 12 ? "passed" : "failed", passedGroups: checks.length, checks, sourceSHA256 }, null, 2) + "\n");
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
