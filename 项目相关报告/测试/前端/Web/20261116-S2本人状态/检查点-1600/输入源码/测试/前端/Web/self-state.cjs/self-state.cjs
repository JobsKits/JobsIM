// Created by Jobs on 2026年11月16日，星期一.
const fs = require("node:fs"), path = require("node:path"), crypto = require("node:crypto");
const { chromium } = require(process.env.JOBSIM_PLAYWRIGHT_MODULE || "playwright");
const root = path.resolve(__dirname, "../../../..");
const out = process.env.JOBSIM_SYNC_REPORT || path.join(root, "项目相关报告/测试/前端/Web/执行记录", "self-state-" + Date.now() + "-" + process.pid);
if (fs.existsSync(out)) throw Error("Choose a new report directory");
fs.mkdirSync(out, { recursive: true });
const sources = fs.readdirSync(path.join(root, "代码/前端/Web/Src")).filter(name => /\.(ts|tsx|css)$/.test(name)).map(name => "代码/前端/Web/Src/" + name);
sources.push("测试/前端/Web/self-state.cjs/self-state.cjs");
const hashes = () => Object.fromEntries(sources.map(file => [file, crypto.createHash("sha256").update(fs.readFileSync(path.join(root, file))).digest("hex")]));
const before = hashes(), checks = [], startedAt = new Date().toISOString();
const assert = (value, reason) => { if (!value) throw Error(reason); };
(async () => {
  const browser = await chromium.launch({ headless: true, executablePath: process.env.BROWSER_EXECUTABLE || "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" });
  const context = await browser.newContext(), page = await context.newPage();
  try {
    await page.route("**/api/**", route => route.abort());
    await page.goto(process.env.JOBSIM_WEB_URL || "http://127.0.0.1:5199", { waitUntil: "domcontentloaded" });
    await page.evaluate(async () => {
      const store = await import("/Src/SyncStore.ts"), protocol = await import("/Src/SelfStateProtocol.ts"), actions = await import("/Src/SelfStateActions.ts"), api = await import("/Src/api.ts"), storage = await import("/Src/storage.ts"), types = await import("/Src/types.ts"), view = await import("/Src/SyncView.ts");
      const db = await storage.storageDatabase;
      const read = key => new Promise((resolve, reject) => { const tx = db.transaction("spaces"), q = tx.objectStore("spaces").get(key); tx.oncomplete = () => resolve(q.result); tx.onerror = () => reject(tx.error); });
      const write = rows => new Promise((resolve, reject) => { const tx = db.transaction("spaces", "readwrite", { durability: "strict" }), s = tx.objectStore("spaces"); for (const [key, value] of rows) s.put(value, key); tx.oncomplete = resolve; tx.onerror = () => reject(tx.error); });
      const create = async () => {
        const f = { scope: crypto.randomUUID(), user: crypto.randomUUID(), device: crypto.randomUUID(), conversation: crypto.randomUUID(), current: true, supported: true, log: [], proofs: {}, drop: false, retired: false };
        f.identity = { scopeId: f.scope, principalId: f.user, deviceId: f.device, sourceContextId: "personal", restoreEpoch: "2" };
        f.namespace = location.origin + "|" + f.scope + "|" + f.user + "|personal|2|" + f.device;
        f.base = () => ({ id: crypto.randomUUID(), scopeId: f.scope, userId: f.user, sourceContextId: "personal", revisionEpoch: "2", conversationId: f.conversation, version: "0" });
        f.preference = { scopeId: f.scope, userId: f.user, sourceContextId: "personal", revisionEpoch: "2", version: "0", fontPreset: "standard", explicit: false };
        f.view = { ...f.base(), pinnedRank: null, mutedUntil: null, archived: false, hidden: false, manualUnread: false, manualUnreadGeneration: "0", manualUnreadMarkedAt: null, manualUnreadFrom: null };
        f.read = { ...f.base(), membershipGeneration: "9007199254740993", readPoint: { epoch: "0", sequence: "0" }, writable: true, countStatus: "exact_current_source", continuityStatus: "complete", unreadCount: "9007199254740997", countSourceWatermark: "9007199254740998" };
        f.draft = { ...f.base(), contentState: "empty", ownedText: null, replyState: "none", reply: null, draftAdmission: "da1.FIXTURE_ORIGINAL", writable: true };
        f.request = async (path, init = {}) => {
          const method = init.method || "GET", body = init.body ? JSON.parse(init.body) : undefined;
          f.log.push({ path, method, raw: init.body, body });
          if (path === "/bootstrap") return { deploymentId: f.scope, features: { sync_self_state: f.supported, cloud_drafts: f.supported } };
          const kind = path.includes("preferences") ? "preferences" : path.includes("/view") ? "view" : path.includes("/read") ? "read" : "draft";
          if (path.endsWith("-results")) {
            const original = kind === "draft" ? body.body : body;
            const proof = f.proofs[original.requestId];
            if (!proof) { if (f.withdrawAfterMissing) f.supported = false; throw new api.ApiError(404, { preferences: "PREFERENCE", view: "VIEW", read: "READ", draft: "DRAFT" }[kind] + "_OPERATION_NOT_FOUND", "fixture missing"); }
            if (proof.raw !== (kind === "draft" ? JSON.stringify(body.body) : init.body) || kind === "draft" && proof.method !== body.method) throw new api.ApiError(409, "IDEMPOTENCY_CONFLICT", "fixture conflict");
            return structuredClone(proof.result);
          }
          if (method === "GET") return structuredClone(kind === "preferences" ? f.preference : f[kind]);
          let result;
          if (kind === "preferences") result = { revisionEpoch: "2", version: "1", fontPreset: body.fontPreset };
          if (kind === "view") {
            const { expectedManualUnreadGeneration, ...changes } = body.changes;
            result = { ...f.view, ...changes, version: "1", manualUnreadGeneration: changes.manualUnread === undefined ? f.view.manualUnreadGeneration : "1" };
          }
          if (kind === "read") result = { revisionEpoch: "2", version: "1", membershipGeneration: body.membershipGeneration, readPoint: { epoch: "0", sequence: "0" }, observedMessageIds: body.observedMessageIds, countStatus: "not_computed", continuityStatus: "gap", hasMoreEligible: true, hasUnorderedEligible: false };
          if (kind === "draft") result = { revisionEpoch: "2", version: "1", contentState: method === "DELETE" ? "empty" : "owned", replyState: body.reply ? "available" : "none" };
          const operation = { operationId: ["read", "draft"].includes(kind) ? crypto.randomUUID() : body.requestId, status: kind === "draft" && f.retired ? "retired" : kind === "draft" && f.prepared ? "prepared" : "applied", current: !f.retired, result };
          if (kind === "view") f.view = structuredClone(result);
          if (kind === "draft" && operation.status === "applied") f.draft = { ...f.draft, ...result, ownedText: method === "DELETE" ? null : body.ownedText, reply: body.reply ? { messageId: body.reply.messageId, sourceVersion: body.reply.sourceVersion } : null };
          f.proofs[body.requestId] = { raw: init.body, method, result: operation };
          if (kind === "draft" && f.retired) f.draft.recovery = { operationId: operation.operationId, ownedText: body.ownedText };
          if (f.drop) { f.drop = false; throw new TypeError("fixture lost response"); }
          return structuredClone(operation);
        };
        await write([["auth-active|" + f.namespace.split("|").slice(0, 4).join("|"), f.namespace], [f.namespace, { ...types.emptySpace(), conversations: [{ id: f.conversation, membershipGeneration: f.read.membershipGeneration }], drafts: { [f.conversation]: "LOCAL_LATEST" } }]]);
        const old = await store.ensureSyncState(f.namespace, f.identity, () => f.current);
        f.state = { ...old, protocolSchema: "2", streamId: crypto.randomUUID(), cursor: "fixture", restriction: undefined };
        await write([["sync-state|" + f.namespace, f.state]]);
        f.actions = () => new actions.SelfStateActions({ namespace: f.namespace, identity: f.identity, isCurrent: () => f.current, request: f.request });
        return f;
      };
      window.selfTest = { store, protocol, actions, api, storage, types, view, db, read, write, create };
    });
    async function test(name, action) {
      assert(await page.evaluate(action), name);
      checks.push({ name, status: "passed" }); console.log("PASS " + name);
    }
    await test("exact string counters and RFC3339 offset stay intact", async () => {
      const t = window.selfTest, f = await t.create(); f.view.mutedUntil = "2026-11-17T20:00:00+08:00";
      const view = await f.actions().get("view", f.conversation), read = await f.actions().get("read", f.conversation);
      return view.mutedUntil === f.view.mutedUntil && read.unreadCount === "9007199254740997" && read.membershipGeneration === "9007199254740993";
    });
    await test("wrong principal live response cannot stage a write", async () => {
      const t = window.selfTest, f = await t.create(); f.preference.userId = crypto.randomUUID();
      let code; try { await f.actions().font("large"); } catch (e) { code = e.code; }
      return code === "SYNC_PERSONAL_IDENTITY_MISMATCH" && !(await t.read("self-state-operations|" + f.namespace)) && !f.log.some(row => row.method === "PATCH");
    });
    await test("lost response resumes exact original body after cold coordinator", async () => {
      const t = window.selfTest, f = await t.create(); f.drop = true;
      try { await f.actions().font("large"); } catch {}
      const [original] = await f.actions().operations(), saved = original.originalBody;
      f.preference.version = "9"; f.preference.fontPreset = "small";
      await t.write([["sync-state|" + f.namespace, { ...f.state, guardGeneration: crypto.randomUUID() }]]);
      const result = await f.actions().resume(original.requestId);
      const [completed] = await f.actions().operations();
      return result.status === "applied" && completed.originalBody === saved && completed.status === "applied" && f.log.filter(row => row.method === "PATCH").length === 1;
    });
    await test("completed action checks current again without rewriting", async () => {
      const t = window.selfTest, f = await t.create(); await f.actions().font("large");
      const [old] = await f.actions().operations(); f.proofs[old.requestId].result.current = false;
      const result = await f.actions().resume(old.requestId, true);
      return result.current === false && f.log.filter(row => row.method === "PATCH").length === 1 && (await f.actions().operations())[0].result.current === false;
    });
    await test("unknown pending result alone never resends", async () => {
      const t = window.selfTest, f = await t.create(); f.withdrawAfterMissing = true;
      try { await f.actions().font("large"); } catch {}
      f.supported = true; f.withdrawAfterMissing = false;
      const [pending] = await f.actions().operations();
      let code; try { await f.actions().resume(pending.requestId); } catch (e) { code = e.code; }
      return code === "PREFERENCE_OPERATION_NOT_FOUND" && !f.log.some(row => row.method === "PATCH") && (await f.actions().operations())[0].status === "pending";
    });
    await test("capability withdrawn between proof and write stops original mutation", async () => {
      const t = window.selfTest, f = await t.create(); f.withdrawAfterMissing = true;
      let code; try { await f.actions().font("large"); } catch (e) { code = e.code; }
      return code === "SYNC_SELF_STATE_UNAVAILABLE" && !f.log.some(row => row.method === "PATCH") && (await f.actions().operations()).length === 1;
    });
    await test("queued old identity cannot borrow a later account", async () => {
      const t = window.selfTest, f = await t.create(); let release, entered;
      const ready = new Promise(resolve => { entered = resolve; });
      const lock = navigator.locks.request("jobsim-self-state|" + f.namespace, async () => { entered(); await new Promise(resolve => { release = resolve; }); });
      await ready; const pending = f.actions().font("large").then(() => "bad", e => e.code); f.current = false; release(); await lock;
      return await pending === "SYNC_CONTEXT_CHANGED" && f.log.length === 0;
    });
    await test("server read UUID is independent and only observed IDs are cleared", async () => {
      const t = window.selfTest, f = await t.create(), seen = crypto.randomUUID(), hole = crypto.randomUUID();
      const space = await t.read(f.namespace); space.unread = { [f.conversation]: [hole, seen] }; space.mentionUnread = { [f.conversation]: [hole, seen] }; await t.write([[f.namespace, space]]);
      const result = await f.actions().observe(f.conversation, [seen]);
      const [operation] = await f.actions().operations(), after = await t.read(f.namespace);
      return result.operationId !== operation.requestId && JSON.stringify(after.unread[f.conversation]) === JSON.stringify([hole]) && JSON.stringify(after.mentionUnread[f.conversation]) === JSON.stringify([hole]);
    });
    await test("manual unread retains independent generation CAS", async () => {
      const t = window.selfTest, f = await t.create(); await f.actions().view(f.conversation, { manualUnread: true });
      const body = JSON.parse((await f.actions().operations())[0].originalBody);
      return body.changes.expectedManualUnreadGeneration === "0" && body.changes.manualUnread === true;
    });
    await test("retired draft is terminal and recovery does not replace local input", async () => {
      const t = window.selfTest, f = await t.create(); f.retired = true;
      const result = await f.actions().saveDraft(f.conversation, "OWN_NEW_TEXT", { messageId: crypto.randomUUID(), sourceVersion: "8", replyAdmission: "dr1.ORIGINAL" });
      const [operation] = await f.actions().operations(), live = await f.actions().get("draft", f.conversation), space = await t.read(f.namespace);
      await f.actions().resume(operation.requestId, true);
      return result.status === "retired" && !result.current && live.recovery.ownedText === "OWN_NEW_TEXT" && space.drafts[f.conversation] === "LOCAL_LATEST" && operation.originalBody.includes("dr1.ORIGINAL") && f.log.filter(row => row.method === "PUT").length === 1;
    });
    await test("prepared draft remains pending and explicit retry retains nonce and proofs", async () => {
      const t = window.selfTest, f = await t.create(); f.prepared = true;
      const first = await f.actions().saveDraft(f.conversation, "OWN_TEXT"); const [old] = await f.actions().operations();
      f.draft.draftAdmission = "da1.LATER";
      const second = await f.actions().resume(old.requestId, true), [newer] = await f.actions().operations();
      return first.status === "prepared" && second.status === "prepared" && newer.status === "prepared" && newer.originalBody === old.originalBody && !newer.originalBody.includes("LATER");
    });
    await test("explicit adopt is local CAS and never changes the message outbox", async () => {
      const t = window.selfTest, f = await t.create();
      let code; try { await t.store.adoptSelfDraft(f.namespace, f.identity, f.conversation, "OLD_LOCAL", "CLOUD", () => true); } catch (e) { code = e.code; }
      const untouched = await t.read(f.namespace);
      await t.store.adoptSelfDraft(f.namespace, f.identity, f.conversation, "LOCAL_LATEST", "CLOUD", () => true);
      const changed = await t.read(f.namespace);
      return code === "SELF_LOCAL_DRAFT_CHANGED" && untouched.drafts[f.conversation] === "LOCAL_LATEST" && changed.drafts[f.conversation] === "CLOUD" && changed.outbox.length === 0 && !(await t.read("outbox|" + f.namespace));
    });
    await test("S1 upgrade archives original responsibility without replacing physical store or queue", async () => {
      const t = window.selfTest, f = await t.create(), original = { ...f.state, protocolSchema: "1", position: "9007199254740993", origins: { marker: { eventEnvelope: "ORIGINAL_RAW" } } };
      const queue = [{ clientMsgId: crypto.randomUUID(), original: "ORIGINAL_SEND_BODY" }];
      await t.write([["sync-state|" + f.namespace, original], ["outbox|" + f.namespace, queue]]);
      const next = await t.store.selectSyncSchema(f.namespace, f.identity, original, "2", () => true);
      const archive = await t.read("sync-schema-archive|" + f.namespace + "|" + original.registrationRequestId);
      return next.storeGeneration === original.storeGeneration && next.registrationRequestId !== original.registrationRequestId && next.position === "0" && archive.origins.marker.eventEnvelope === "ORIGINAL_RAW" && JSON.stringify(await t.read("outbox|" + f.namespace)) === JSON.stringify(queue);
    });
    await test("personal metadata and cursor roll back together on wrong scope", async () => {
      const t = window.selfTest, f = await t.create(), state = await t.read("sync-state|" + f.namespace);
      const ref = { owner: "directory", kind: "preferences", id: f.user, subjectUserId: f.user, revisionEpoch: "2", revision: "7", state: "active" };
      let code;
      try { await t.store.commitSyncProjection({ namespace: f.namespace, identity: f.identity, binding: { ...f.identity, storeGeneration: state.storeGeneration, streamId: state.streamId }, expectedGuardGeneration: state.guardGeneration, expectedCursor: state.cursor, cursor: "BAD_NEW_CURSOR", position: "1", permissionGeneration: "f".repeat(64), classificationWatermark: "1", projections: [{ ref, state: "available", revisionMatch: true, currentRevision: "7", currentRevisionEpoch: "2", object: { ...f.preference, scopeId: crypto.randomUUID() } }], materialize: value => value, isCurrent: () => true }); } catch (e) { code = e.code; }
      return code === "SYNC_PERSONAL_IDENTITY_MISMATCH" && (await t.read("sync-state|" + f.namespace)).cursor === "fixture" && !(await t.read("sync-personal|" + f.namespace));
    });
    await test("owner clock stays separate from object version and raw origin conflicts roll back", async () => {
      const t = window.selfTest, f = await t.create(), origin = crypto.randomUUID();
      const ref = { owner: "directory", kind: "preferences", id: f.user, subjectUserId: f.user, revisionEpoch: "2", revision: "9007199254740993", state: "active" };
      const projection = { ref, state: "available", revisionMatch: true, currentRevision: ref.revision, currentRevisionEpoch: "2", object: { ...f.preference, version: "1" } };
      const event = { kind: "reference", position: "1", originEventId: origin, eventDigest: "f".repeat(64), eventEnvelope: "ORIGINAL_ENVELOPE", ref, projection };
      const commit = async (selected, cursor) => {
        const state = await t.read("sync-state|" + f.namespace);
        return t.store.commitSyncProjection({ namespace: f.namespace, identity: f.identity, binding: { ...f.identity, storeGeneration: state.storeGeneration, streamId: state.streamId }, expectedGuardGeneration: state.guardGeneration, expectedCursor: state.cursor, cursor, position: "1", permissionGeneration: "f".repeat(64), classificationWatermark: "1", projections: [projection], events: [selected], materialize: value => value, isCurrent: () => true });
      };
      await commit(event, "CURSOR_ONE"); let code;
      try { await commit({ ...event, eventDigest: "a".repeat(64) }, "BAD_CONFLICT_CURSOR"); } catch (e) { code = e.code; }
      const state = await t.read("sync-state|" + f.namespace), personal = await t.read("sync-personal|" + f.namespace);
      return code === "SYNC_ORIGIN_CONFLICT" && state.cursor === "CURSOR_ONE" && state.origins[origin].eventEnvelope === "ORIGINAL_ENVELOPE" && personal.records["preferences|" + f.user].version === "1";
    });
    await test("reply admissions are excluded from ordinary persisted message views", async () => {
      const t = window.selfTest, f = await t.create(), message = { id: crypto.randomUUID(), conversationId: f.conversation, version: "1", replyAdmission: "dr1.NEVER_EXPORT" };
      const value = t.storage.persistableSpace({ ...t.types.emptySpace(), messages: { [f.conversation]: [message] }, outbox: [message] });
      return !JSON.stringify(value).includes("NEVER_EXPORT") && !Object.hasOwn(value.messages[f.conversation][0], "replyAdmission") && !Object.hasOwn(value.outbox[0], "replyAdmission");
    });
    await test("all ten languages provide every own-state UI label", async () => {
      const { selfStateLabels } = await import("/Src/SelfStateLabels.ts");
      const keys = Object.keys(selfStateLabels.en);
      return Object.keys(selfStateLabels).length === 10 && Object.values(selfStateLabels).every(row => Object.keys(row).length === keys.length && keys.every(key => typeof row[key] === "string" && row[key].trim()));
    });
    await page.evaluate(async () => {
      const resources = performance.getEntriesByType("resource").map(row => row.name);
      const reactURL = resources.find(url => /\/react\.js\?/.test(url)), domURL = resources.find(url => /\/react-dom_client\.js\?/.test(url));
      if (!reactURL || !domURL) throw Error("Real application React modules not found");
      const reactModule = await import(reactURL), domModule = await import(domURL), { default: Panel } = await import("/Src/SelfStatePanel.tsx"), { translate } = await import("/Src/i18n.ts");
      const React = reactModule.default ?? reactModule, dom = domModule.default ?? domModule;
      const f = await window.selfTest.create(); window.panelFixture = f; f.adoptions = []; f.errors = []; f.draft.ownedText = "CLOUD_INDEPENDENT_TEXT"; f.draft.contentState = "owned";
      const container = document.createElement("div"); container.id = "self-panel-fixture"; container.style.cssText = "max-width:430px;margin:auto;background:var(--surface);color:var(--text)";
      document.getElementById("root").style.display = "none"; document.body.append(container);
      const actions = f.actions(); f.locale = "en";
      const render = () => domRoot.render(React.createElement(Panel, { actions, conversationId: f.conversation, locale: f.locale, localDraft: "LOCAL_LATEST", selectedReply: null, cloudEnabled: true, t: key => translate(f.locale, key), onAdopt: async (text, expected) => { f.adoptions.push({ text, expected }); }, onChanged: () => {}, onError: error => f.errors.push(String(error.code || error.message)) }));
      const domRoot = dom.createRoot(container); f.render = render; render();
    });
    const panel = page.getByRole("region", { name: "My synced state" });
    await panel.getByText("CLOUD_INDEPENDENT_TEXT", { exact: true }).waitFor();
    assert(await page.evaluate(() => window.panelFixture.adoptions.length === 0), "cloud GET automatically adopted text");
    await panel.getByRole("checkbox", { name: "Pin", exact: true }).click();
    await panel.getByRole("checkbox", { name: "Pin", exact: true }).waitFor({ state: "visible" });
    await page.waitForFunction(() => window.panelFixture.view.pinnedRank === 0);
    assert(await panel.getByRole("checkbox", { name: "Pin", exact: true }).isChecked(), "pin UI did not reflect current view");
    checks.push({ name: "actual panel controls issue a view intent and reflect reload", status: "passed" });
    await panel.getByRole("button", { name: "Save to cloud", exact: true }).click();
    await page.waitForFunction(() => window.panelFixture.draft.ownedText === "LOCAL_LATEST");
    assert(await page.evaluate(() => window.panelFixture.adoptions.length === 0), "cloud save adopted text");
    await panel.getByRole("button", { name: "Clear cloud draft", exact: true }).click();
    await page.waitForFunction(() => window.panelFixture.draft.contentState === "empty");
    assert(await page.evaluate(() => window.panelFixture.adoptions.length === 0 && window.panelFixture.errors.length === 0), "clear modified local editor or panel error");
    checks.push({ name: "actual cloud save and clear controls leave the local editor untouched", status: "passed" });
    await panel.getByRole("button", { name: "Use this text", exact: true }).click();
    await page.waitForFunction(() => window.panelFixture.adoptions.length === 1);
    assert(await page.evaluate(() => window.panelFixture.adoptions[0].expected === "LOCAL_LATEST" && window.panelFixture.adoptions[0].text === ""), "explicit adopt lacked captured local CAS value");
    checks.push({ name: "actual adopt control explicitly supplies captured local editor value", status: "passed" });
    await page.setViewportSize({ width: 390, height: 844 });
    for (const [locale, theme] of [["zh-CN", "light"], ["bo", "dark"], ["es", "light"]]) {
      await page.evaluate(({ locale, theme }) => { window.panelFixture.locale = locale; document.documentElement.dataset.theme = theme; window.panelFixture.render(); }, { locale, theme });
      await page.waitForTimeout(100);
      assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), "panel overflow " + locale);
      await page.screenshot({ path: path.join(out, "界面-" + locale + "-" + theme + ".png"), fullPage: true });
    }
    checks.push({ name: "actual panel mobile layouts captured in Chinese Tibetan Spanish and light dark themes", status: "passed" });
    const after = hashes(); assert(JSON.stringify(before) === JSON.stringify(after), "source changed while testing");
    fs.writeFileSync(path.join(out, "结果.json"), JSON.stringify({ startedAt, finishedAt: new Date().toISOString(), status: "passed", scope: "Real Chrome IndexedDB/Web Locks; injected HTTP fixtures; all /api requests aborted. No live server, native fsync qualification or receipt ACK.", checks, sourceSHA256: before, sourceUnchanged: true }, null, 2));
  } catch (error) {
    await page.screenshot({ path: path.join(out, "当场失败.png"), fullPage: true }).catch(() => {});
    fs.writeFileSync(path.join(out, "结果.json"), JSON.stringify({ startedAt, status: "failed", checks, error: String(error.stack || error), sourceSHA256: before, afterSHA256: hashes() }, null, 2));
    throw error;
  } finally { await context.close(); await browser.close(); }
})().catch(error => { console.error(error); process.exitCode = 1; });
