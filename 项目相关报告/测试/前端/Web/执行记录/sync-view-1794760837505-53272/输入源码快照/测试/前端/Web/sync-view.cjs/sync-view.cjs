// Created by Jobs on 2026年11月16日，星期一.
const fs = require("node:fs"), path = require("node:path"), crypto = require("node:crypto");
const { chromium } = require(process.env.JOBSIM_PLAYWRIGHT_MODULE || "playwright");
const root = path.resolve(__dirname, "../../../..");
const out = process.env.JOBSIM_SYNC_VIEW_REPORT || path.join(root, "项目相关报告/测试/前端/Web/执行记录", "sync-view-" + Date.now() + "-" + process.pid);
if (fs.existsSync(out)) throw Error("Choose a new report directory");
fs.mkdirSync(out, { recursive: true });
const files = ["SyncView.ts", "SyncStore.ts", "SyncEngine.ts", "SyncProtocol.ts", "storage.ts", "types.ts", "forward.ts"].map(name => "代码/前端/Web/Src/" + name);
files.push("测试/前端/Web/sync-view.cjs/sync-view.cjs");
const fingerprints = () => Object.fromEntries(files.map(name => [name, crypto.createHash("sha256").update(fs.readFileSync(path.join(root, name))).digest("hex")]));
const sourceSHA256 = fingerprints(), at = new Date().toISOString();
let results = [], status = "failed", networkAPIRequests = 0;
(async () => {
  const browser = await chromium.launch({ headless: true, executablePath: process.env.BROWSER_EXECUTABLE || "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" });
  try {
    const context = await browser.newContext(), page = await context.newPage();
    await page.route("**/api/**", route => { networkAPIRequests++; return route.abort(); });
    await page.goto(process.env.JOBSIM_WEB_URL || "http://127.0.0.1:5189", { waitUntil: "domcontentloaded" });
    results = await page.evaluate(async () => {
      const { materializeSyncSpace: map, refreshSyncForwardViews: refresh } = await import("/Src/SyncView.ts");
      const storage = await import("/Src/storage.ts"), store = await import("/Src/SyncStore.ts"), engine = await import("/Src/SyncEngine.ts");
      const { emptySpace } = await import("/Src/types.ts"), { messagePayload } = await import("/Src/forward.ts");
      const db = await storage.storageDatabase, results = [];
      const assert = (value, name) => { if (!value) throw Error(name); };
      const read = key => new Promise((resolve, reject) => { const tx = db.transaction("spaces"), r = tx.objectStore("spaces").get(key); tx.oncomplete = () => resolve(r.result); tx.onerror = () => reject(tx.error); });
      const write = rows => new Promise((resolve, reject) => { const tx = db.transaction("spaces", "readwrite", { durability: "strict" }), s = tx.objectStore("spaces"); for (const [k, v] of rows) s.put(v, k); tx.oncomplete = () => resolve(); tx.onerror = () => reject(tx.error); });
      const test = async (name, run) => { try { await run(); results.push({ name, status: "passed" }); } catch (e) { results.push({ name, status: "failed", error: e.message }); } };
      const fixture = () => {
        const user = crypto.randomUUID(), peer = crypto.randomUUID(), conversation = crypto.randomUUID(), client = crypto.randomUUID(), id = crypto.randomUUID();
        const c = { id: conversation, kind: "group", title: "Scoped view fixture", ownerId: user, memberIds: [user, peer], updatedAt: "2026-11-16T00:00:00Z", cursor: "0" };
        const m = { id, conversationId: conversation, senderId: peer, clientMsgId: client, type: "text", text: "SCOPED_BODY", createdAt: "2026-11-16T00:00:00Z", updatedAt: "2026-11-16T00:00:00Z", version: "1", sequence: "1", deleted: false, reactions: [], pinned: false, favorite: false };
        const pending = { ...m, id: crypto.randomUUID(), senderId: user, pending: true, sendAttempted: true, text: "ORIGINAL_PENDING", restoreEpoch: "1", sendAdmission: "original-admission" };
        const space = { ...emptySpace(), conversations: [c], drafts: { [conversation]: "ORIGINAL_DRAFT" }, messages: {}, outbox: [pending] };
        const projection = (object, state = "available") => ({ ref: { owner: "message", kind: "message", id: object.id, revision: "1", revisionEpoch: "1", conversationId: conversation, subjectUserId: object.senderId || peer, state: "active" }, state, currentRevision: "1", currentRevisionEpoch: "1", revisionMatch: true, object });
        const conversationProjection = { ref: { owner: "conversation", kind: "conversation", id: c.id, revision: "1", revisionEpoch: "1", conversationId: c.id, state: "active" }, state: "available", currentRevision: "1", currentRevisionEpoch: "1", revisionMatch: true, object: c };
        const mode = { replaceCloud: false, currentUserId: user };
        return { user, peer, conversation, client, c, m, pending, space, projection, conversationProjection, mode };
      };
      await test("sync self message alone never acknowledges original Outbox", () => {
        const f = fixture(), before = JSON.stringify(f.pending), next = map(f.space, [f.projection({ ...f.m, senderId: f.user })], f.mode);
        assert(next.outbox.length === 1 && JSON.stringify(next.outbox[0]) === before && JSON.stringify(f.pending) === before, "sync bypassed original-body proof or changed intent");
        assert(next.messages[f.conversation].filter(m => m.pending).length === 1, "unproven pending bubble hidden");
      });
      await test("peer forged clientId never clears or hides local pending", () => {
        const f = fixture(), next = map(f.space, [f.projection(f.m)], f.mode);
        assert(next.outbox.length === 1 && next.messages[f.conversation].some(m => m.pending && m.senderId === f.user), "peer clientId hid pending");
        assert(next.messages[f.conversation].some(m => m.id === f.m.id && m.senderId === f.peer), "peer message lost");
      });
      await test("snapshot history is not all-new local unread", () => {
        const f = fixture(), next = map({ ...f.space, outbox: [] }, [f.conversationProjection, f.projection({ ...f.m, mentionedMe: true })], { ...f.mode, replaceCloud: true });
        assert(!next.unread[f.conversation]?.length && !next.mentionUnread[f.conversation]?.length, "initial snapshot marked historical unread");
      });
      await test("new peer delta adds unread and structured mention once", () => {
        const f = fixture(), p = f.projection({ ...f.m, mentionedMe: true }), first = map({ ...f.space, outbox: [] }, [p], f.mode), again = map(first, [p], f.mode);
        assert(first.unread[f.conversation]?.length === 1 && first.mentionUnread[f.conversation]?.length === 1 && again.unread[f.conversation]?.length === 1 && again.mentionUnread[f.conversation]?.length === 1, "delta unread or mention duplicated");
      });
      await test("currently read conversation clears old and new local unread", () => {
        const f = fixture(), before = { ...f.space, outbox: [], messages: { [f.conversation]: [f.m] }, unread: { [f.conversation]: [f.m.id] }, mentionUnread: { [f.conversation]: [f.m.id] } };
        const next = map(before, [f.projection({ ...f.m, id: crypto.randomUUID(), mentionedMe: true })], { ...f.mode, readingConversationId: f.conversation });
        assert(next.unread[f.conversation].length === 0 && next.mentionUnread[f.conversation].length === 0, "reading conversation gained unread");
      });
      await test("own delta and old edits do not create new unread", () => {
        const f = fixture(), before = { ...f.space, outbox: [], messages: { [f.conversation]: [f.m] } };
        const next = map(before, [f.projection({ ...f.m, version: "2", mentionedMe: true }), f.projection({ ...f.m, id: crypto.randomUUID(), senderId: f.user, mentionedMe: true })], f.mode);
        assert(!next.unread[f.conversation]?.length && !next.mentionUnread[f.conversation]?.length, "own message or edit marked unread");
      });
      await test("minimal global object clears body and sidecars", () => {
        const f = fixture(), previous = { ...f.m, attachmentId: crypto.randomUUID(), attachmentURL: "/api/media/private", forwardAdmission: "temporary-proof", replyToId: crypto.randomUUID(), pinned: true, favorite: true };
        const next = map({ ...f.space, outbox: [], messages: { [f.conversation]: [previous] }, unread: { [f.conversation]: [previous.id] } }, [f.projection({ id: f.m.id, version: "2", deleted: true }, "global")], f.mode), row = next.messages[f.conversation][0];
        assert(row.deleted && !row.text && !row.attachmentId && !row.attachmentURL && !row.forwardAdmission && !row.replyToId && !row.pinned && !row.favorite && next.unread[f.conversation].length === 0, "minimal global required full body or retained sidecar");
      });
      await test("known global cannot be reopened by same old newer or unknown source", () => {
        const f = fixture(), previous = { ...f.m, deleted: true, text: "", version: "2" }, before = { ...f.space, outbox: [], messages: { [f.conversation]: [previous] } };
        for (const version of ["1", "2", "3"]) {
          const next = map(before, [f.projection({ ...f.m, version })], f.mode); assert(next.messages[f.conversation][0].deleted && !next.messages[f.conversation][0].text, "late available reopened global");
        }
        const unknown = map(before, [{ ...f.projection(f.m, "unknown"), object: undefined }], f.mode);
        assert(unknown.messages[f.conversation][0].deleted, "unknown erased known global classification");
      });
      await test("fresh forward stays in RAM but durable view only contains placeholder", () => {
        const f = fixture(), next = map({ ...f.space, outbox: [] }, [f.projection({ ...f.m, forwarded: true, text: "FRESH_FORWARD", attachmentURL: "/api/messages/" + f.m.id + "/attachment", forwardAdmission: "temporary-proof" })], f.mode), saved = storage.persistableSpace(next);
        assert(next.messages[f.conversation][0].text === "FRESH_FORWARD" && !saved.messages[f.conversation][0].text && !saved.messages[f.conversation][0].attachmentURL && !saved.messages[f.conversation][0].forwardAdmission && !saved.messages[f.conversation][0].deleted, "forward caching or false global");
      });
      await test("late exact source refresh cannot reopen current global row", async () => {
        const f = fixture(), cached = { ...f.m, forwarded: true }, current = { ...cached, deleted: true, text: "", version: "2" };
        const next = await refresh({ ...f.space, outbox: [], messages: { [f.conversation]: [current] } }, [cached], async () => cached, () => true);
        assert(next.messages[f.conversation][0].deleted && !next.messages[f.conversation][0].text, "late exact source reopened global");
      });

      const durable = async () => {
        const f = fixture(), deviceId = crypto.randomUUID(), scopeId = crypto.randomUUID(), streamId = crypto.randomUUID();
        f.identity = { scopeId, principalId: f.user, sourceContextId: "personal", restoreEpoch: "1", deviceId };
        f.namespace = location.origin + "|" + scopeId + "|" + f.user + "|personal|1|" + deviceId;
        await write([["auth-active|" + f.namespace.split("|").slice(0, 4).join("|"), f.namespace], [f.namespace, f.space], ["outbox|" + f.namespace, [f.pending]]]);
        let state = await store.ensureSyncState(f.namespace, f.identity, () => true);
        state = await store.bindSyncStream(f.namespace, f.identity, state, streamId, () => true);
        f.state = { ...state, cursor: "before-cursor", position: "0", permissionGeneration: "a".repeat(64), classificationWatermark: "1" };
        await write([["sync-state|" + f.namespace, f.state]]);
        f.binding = { ...f.identity, storeGeneration: state.storeGeneration, streamId };
        f.proof = { clientMsgId: f.pending.clientMsgId, conversationId: f.conversation, originalBody: messagePayload(f.pending) };
        f.commit = proofs => store.commitSyncProjection({ namespace: f.namespace, identity: f.identity, binding: f.binding, expectedGuardGeneration: f.state.guardGeneration, expectedCursor: f.state.cursor, cursor: "after-cursor", position: "1", permissionGeneration: f.state.permissionGeneration, classificationWatermark: "1", projections: [f.projection({ ...f.m, senderId: f.user })], outboxProofs: proofs, isCurrent: () => true, materialize: (previous, rows, mode) => map(previous, rows, { ...mode, currentUserId: f.user }) });
        return f;
      };
      await test("exact original body route and owner proof clears only acknowledged Outbox", async () => {
        const f = await durable(), before = JSON.stringify(f.pending); await f.commit([f.proof]);
        const queue = await read("outbox|" + f.namespace), state = await read("sync-state|" + f.namespace);
        assert(queue.length === 0 && state.cursor === "after-cursor" && JSON.stringify(f.pending) === before, "exact proof not atomically applied");
      });
      await test("same clientId with changed text admission or route never acknowledges", async () => {
        for (const mismatch of ["text", "admission", "route"]) {
          const f = await durable(), proof = structuredClone(f.proof);
          if (mismatch === "text") proof.originalBody.text = "DIFFERENT_BODY";
          if (mismatch === "admission") proof.originalBody.sendAdmission = "new-admission";
          if (mismatch === "route") proof.conversationId = crypto.randomUUID();
          await f.commit([proof]); const queue = await read("outbox|" + f.namespace);
          assert(queue.length === 1 && JSON.stringify(messagePayload(queue[0])) === JSON.stringify(messagePayload(f.pending)), "mismatched proof erased original");
        }
      });
      await test("foreign sender and missing proof keep original queue", async () => {
        const f = await durable(); await f.commit([]); assert((await read("outbox|" + f.namespace)).length === 1, "no proof acknowledged");
        const other = await durable(), foreign = { ...other.pending, senderId: other.peer };
        await write([["outbox|" + other.namespace, [foreign]]]); await other.commit([other.proof]);
        assert((await read("outbox|" + other.namespace)).length === 1, "foreign sender proof acknowledged");
      });
      await test("Engine callback sees original body and latest-queue CAS rejects changed payload", async () => {
        const f = await durable(), original = JSON.stringify(messagePayload(f.pending)), routes = []; let sawOriginal = false;
        const request = async (url, options = {}) => {
          routes.push(url);
          if (url === "/bootstrap") return { deploymentId: f.identity.scopeId, restoreEpoch: "1", features: { authoritative_sync: true } };
          if (url === "/sync/device") return { ...f.binding, qualification: "transport_only", qualifiedAck: false };
          if (url === "/sync/context") return { ...f.binding, head: "0", permissionGeneration: f.state.permissionGeneration, qualification: "transport_only", qualifiedAck: false, afterReceiptEnabled: false };
          if (url.startsWith("/sync/changes?")) return { items: [], cursor: "fresh-cursor", position: "0", head: "0", hasMore: false, restoreEpoch: "1", permissionGeneration: f.state.permissionGeneration, snapshotRequired: false, qualifiedAck: false };
          throw Error("Unexpected creation route");
        };
        await engine.runDurableSync({ namespace: f.namespace, binding: f.identity, request, isCurrent: () => true, materialize: (space, rows, mode) => map(space, rows, { ...mode, currentUserId: f.user }), reconcileOutbox: async rows => {
          sawOriginal = JSON.stringify(messagePayload(rows[0])) === original;
          await write([["outbox|" + f.namespace, [{ ...f.pending, text: "LATEST_PENDING_BODY" }]]]); return [f.proof];
        } });
        const queue = await read("outbox|" + f.namespace);
        assert(sawOriginal && queue.length === 1 && queue[0].text === "LATEST_PENDING_BODY" && !routes.some(url => /\/messages$/.test(url)), "callback mutated or resent queue");
      });
      return results;
    });
    status = results.every(row => row.status === "passed") && JSON.stringify(sourceSHA256) === JSON.stringify(fingerprints()) ? "passed" : "failed";
    for (const row of results) console.log(row.status.toUpperCase() + " " + row.name + (row.error ? ": " + row.error : ""));
  } finally {
    await browser.close();
    fs.writeFileSync(path.join(out, "结果.json"), JSON.stringify({ at, status, scope: "Real Chrome modules/IndexedDB with simulated projections and proof callback; all /api network blocked; not real backend reconciliation", passedGroups: results.filter(row => row.status === "passed").length, failedGroups: results.filter(row => row.status === "failed").length, blockedPageBootstrapAPIRequests: networkAPIRequests, checks: results, sourceSHA256 }, null, 2) + "\n");
  }
  if (status !== "passed") process.exitCode = 1;
})().catch(error => { console.error(error); process.exitCode = 1; });
