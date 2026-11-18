// Created by Jobs on 2026年11月17日，星期二.
// Real Chrome/React/IndexedDB; API responses and authentication are explicit fixtures.
const fs = require("node:fs"), path = require("node:path"), os = require("node:os"), crypto = require("node:crypto"), { pathToFileURL } = require("node:url");
const { chromium } = require(process.env.JOBSIM_PLAYWRIGHT_MODULE || "playwright");
const root = path.resolve(__dirname, "../../../.."), web = path.join(root, "代码/前端/Web");
const out = process.env.JOBSIM_RECEIPT_REPORT || path.join(root, "项目相关报告/测试/前端/Web/执行记录", "receipt-context-" + Date.now() + "-" + process.pid);
const free = () => { const s = fs.statfsSync(root); return Number(s.bavail) * Number(s.bsize); };
if (free() < 4 * 1024 ** 3) throw Error("4GiB disk gate");
if (fs.existsSync(out)) throw Error("Refuse to overwrite evidence");
fs.mkdirSync(out, { recursive: true });
const sources = fs.readdirSync(path.join(web, "Src")).filter(name => /\.(tsx?|css)$/.test(name)).map(name => "代码/前端/Web/Src/" + name).concat(path.relative(root, __filename));
const hashes = () => Object.fromEntries(sources.map(name => [name, crypto.createHash("sha256").update(fs.readFileSync(path.join(root, name))).digest("hex")]));
const before = hashes(), checks = [], startedAt = new Date().toISOString();
(async () => {
  const work = fs.mkdtempSync(path.join(os.tmpdir(), "JobsIM-ReceiptContext-"));
  const { createServer } = await import(pathToFileURL(path.join(web, "node_modules/vite/dist/node/index.js")).href);
  const vite = await createServer({ root: web, configFile: false, publicDir: "Public", cacheDir: path.join(work, "Vite"), server: { host: "127.0.0.1", port: 0 } });
  let browser;
  try {
    await vite.listen(); browser = await chromium.launch({ headless: true, executablePath: process.env.BROWSER_EXECUTABLE || "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" });
    const page = await browser.newPage(), pageErrors = []; page.on("pageerror", error => pageErrors.push(String(error)));
    await page.route("**/api/**", route => route.abort()); await page.goto(vite.resolvedUrls.local[0], { waitUntil: "domcontentloaded" });
    await page.evaluate(async () => {
      const protocol = await import("/Src/ReceiptContext.ts"), labels = await import("/Src/ReceiptLabels.ts"), auth = await import("/Src/AuthSession.ts"), database = await import("/Src/StorageDatabase.ts"), api = await import("/Src/api.ts"), panel = await import("/Src/ReceiptPanel.tsx"), json = await import("/Src/MigrationArchive.ts");
      const resources = performance.getEntriesByType("resource").map(item => item.name), react = resources.find(value => /\/react\.js\?/.test(value)), dom = resources.find(value => /\/react-dom_client\.js\?/.test(value));
      const R = await import(react), D = await import(dom), React = R.default ?? R, ReactDOM = D.default ?? D;
      const db = await database.storageDatabase;
      const write = (key, value) => new Promise((resolve, reject) => { const tx = db.transaction("spaces", "readwrite", { durability: "strict" }); tx.objectStore("spaces").put(value, key); tx.oncomplete = resolve; tx.onerror = () => reject(tx.error); });
      const read = key => new Promise((resolve, reject) => { const tx = db.transaction("spaces"), request = tx.objectStore("spaces").get(key); tx.oncomplete = () => resolve(request.result); tx.onerror = () => reject(tx.error); });
      const create = async (registered = true) => {
        const f = { current: true, log: [], mode: "missing", declarations: [], ready: true };
        const i = f.identity = { scopeId: crypto.randomUUID(), userId: crypto.randomUUID(), username: "receipt_fixture", deviceId: crypto.randomUUID(), epoch: "3", familyId: crypto.randomUUID() };
        i.namespace = [location.origin, i.scopeId, i.userId, "personal", i.epoch, i.deviceId].join("|"); f.store = crypto.randomUUID();
        f.install = (family = i.familyId) => { auth.configureAuthProtocol(true, i.scopeId, true); auth.rememberPublicAuth(auth.parsePublicAuth({ familyId: family, refreshGeneration: "1", sourceContextId: "personal", restoreEpoch: i.epoch, deviceId: i.deviceId, user: { id: i.userId, username: i.username }, accessExpiresAt: new Date(Date.now() + 100000).toISOString(), idleExpiresAt: new Date(Date.now() + 200000).toISOString(), absoluteExpiresAt: new Date(Date.now() + 300000).toISOString() })); }; f.install();
        f.state = { schema: "1", protocolSchema: "2", identity: { scopeId: i.scopeId, principalId: i.userId, sourceContextId: "personal", deviceId: i.deviceId, restoreEpoch: i.epoch }, storeGeneration: f.store, registrationRequestId: crypto.randomUUID(), guardGeneration: crypto.randomUUID(), streamId: crypto.randomUUID(), position: "9007199254740993", cursor: "old-transport-cursor", projections: {} };
        await write("auth-active|" + i.namespace.split("|").slice(0, 4).join("|"), i.namespace);
        await write(i.namespace, { conversations: [], messages: {}, outbox: [], drafts: {}, cursors: {} });
        if (registered) await write("sync-state|" + i.namespace, f.state);
        f.storage = () => ({ qualificationId: "", storeGeneration: f.store, restoreEpoch: i.epoch, profileId: "", buildDigest: "", verificationDigest: "", storageContract: "", qualified: false, objectQualified: false });
        f.reply = () => ({ protocol: protocol.receiptProtocol, scopeId: i.scopeId, sourceContextId: "personal", principalId: i.userId, deviceId: i.deviceId, storeGeneration: f.store, restoreEpoch: i.epoch, schemaVersion: "2", qualification: "transport_only", storage: f.storage(), qualifiedAck: false, contentReceiptEnabled: false, objectReceiptEnabled: false, afterReceiptEnabled: false, afterObjectReceiptEnabled: false, defaultRetentionMode: "cloud_history", deletionEligible: false, targetRangeComplete: false, readiness: { transportConfigured: true, publishConnected: true, consumeConnected: true, messageDurable: true, journalDurable: true, journalIndependent: true, epochMatches: true, ready: true, blockers: [] } });
        f.body = () => ({ requestId: crypto.randomUUID(), storeGeneration: f.store, revisionEpoch: i.epoch, profileId: "fixture-native-build", buildDigest: "a".repeat(64), verificationDigest: "b".repeat(64) });
        f.request = async (route, options = {}) => {
          const body = options.body ? JSON.parse(options.body) : undefined, method = options.method ?? "GET";
          f.log.push({ route, body, method, store: new Headers(options.headers).get("X-JobsIM-Store-Generation") });
          if (f.before) await f.before(route);
          if (route.startsWith("/sync/receipt-context?")) return f.reply();
          if (route === "/sync/storage-results") {
            const rows = await read("receipt-queries|" + i.namespace); if (!rows?.some(row => JSON.stringify(row.body) === JSON.stringify(body))) throw Error("HTTP before durable original");
            if (f.mode === "lost") throw new api.ApiError(503, "FIXTURE_UNKNOWN", "fixture");
            if (f.mode === "missing") throw new api.ApiError(404, "STORAGE_DECLARATION_NOT_FOUND", "fixture");
            const result = { qualificationId: crypto.randomUUID(), storeGeneration: body.storeGeneration, restoreEpoch: body.revisionEpoch, profileId: body.profileId, buildDigest: body.buildDigest, verificationDigest: body.verificationDigest, storageContract: "native_sqlite_full_v1", ...(!f.legacyDeclaration ? { qualified: true, objectQualified: false } : {}) };
            if (f.resultPoison === "partial") delete result.objectQualified;
            if (f.resultPoison === "false") result.qualified = false;
            if (f.resultPoison === "contract") result.objectQualified = true;
            if (f.resultPoison === "unknown") result.secret = "FORBIDDEN_FIXTURE_FIELD";
            return { result, current: false, storage: f.storage(), deletionEligible: false, afterReceiptEnabled: false };
          }
          if (route.startsWith("/sync/receipt-targets/")) return { protocol: protocol.receiptProtocol, messageId: route.split("/")[3].split("?")[0], qualificationId: new URL(route, location.origin).searchParams.get("qualificationId") ?? "", contentVersion: "9007199254740993", restoreEpoch: i.epoch, deliveryGeneration: "9007199254740993", manifestDigest: "c".repeat(64), receiptScope: "all_target_devices", originalTarget: false, contentTarget: false, objectTarget: false, contentRecorded: false, objectRecorded: false, contentCompleteAt: null, objectCompleteAt: null, current: true, messageCurrent: true, deletionEligible: false };
          throw Error("Creating or receipt-writing HTTP is forbidden: " + route);
        };
        f.context = { identity: i, isCurrent: () => f.current, request: f.request }; f.actions = () => new protocol.ReceiptContextActions(f.context); return f;
      };
      document.getElementById("root").style.display = "none"; const holder = document.createElement("div"); holder.id = "receipt-fixture"; holder.style.cssText = "width:min(640px,calc(100vw - 32px));margin:20px auto"; document.body.append(holder);
      window.receiptTest = { protocol, labels, json, create, write, read, React, dom: ReactDOM.createRoot(holder), panel };
    });
    const test = async (name, fn) => { if (!await fn()) throw Error(name); checks.push(name); };
    await test("ready server keeps unverified browser transport-only and leaves old sync cursor unchanged", () => page.evaluate(async () => { const t = window.receiptTest, f = await t.create(), value = await f.actions().load(), after = await t.read("sync-state|" + f.identity.namespace); return value.readiness.ready && !value.storage.qualified && !t.protocol.browserReceiptCapability.qualifiedAck && !t.protocol.browserReceiptCapability.afterReceiptEnabled && JSON.stringify(after) === JSON.stringify(f.state) && f.log.length === 1 && f.log[0].store === f.store && f.log[0].route.endsWith("schemaVersion=2"); }));
    await test("approved remote native profile cannot grant browser equivalent durability", () => page.evaluate(async () => { const t = window.receiptTest, f = await t.create(), reply = f.reply(); Object.assign(reply, { qualification: "qualified_content_and_objects", qualifiedAck: true, contentReceiptEnabled: true, objectReceiptEnabled: true, afterReceiptEnabled: true, afterObjectReceiptEnabled: true }); Object.assign(reply.storage, { qualificationId: crypto.randomUUID(), profileId: "approved-native", buildDigest: "a".repeat(64), verificationDigest: "b".repeat(64), storageContract: "native_sqlite_full_file_fsync_v1", qualified: true, objectQualified: true }); f.reply = () => reply; const value = await f.actions().load(); return value.storage.objectQualified && !t.protocol.browserReceiptCapability.localDurabilityVerified && f.log.every(row => row.method === "GET"); }));
    await test("inconsistent readiness and changed server store are rejected", () => page.evaluate(async () => { const f = await window.receiptTest.create(), original = f.reply(); let rejected = 0; f.reply = () => ({ ...original, readiness: { ...original.readiness, journalIndependent: false } }); await f.actions().load().catch(() => rejected++); f.reply = () => ({ ...original, storeGeneration: crypto.randomUUID() }); await f.actions().load().catch(() => rejected++); return rejected === 2; }));
    await test("missing registration is never fabricated and sends no negotiation HTTP", () => page.evaluate(async () => { const t = window.receiptTest, f = await t.create(false); let rejected = false; await f.actions().load().catch(() => rejected = true); return rejected && !f.log.length && await t.read("sync-state|" + f.identity.namespace) === undefined; }));
    await test("original six fields commit before pure lookup; 404 creates no declaration", () => page.evaluate(async () => { const f = await window.receiptTest.create(), body = f.body(), result = await f.actions().checkOriginal(body), rows = await f.actions().rows(); return result === undefined && rows[0].state === "not_found" && rows[0].body.requestId === body.requestId && f.log.length === 1 && f.log[0].route === "/sync/storage-results"; }));
    await test("unknown preserves same body and nonce; legacy7/new9 historical facts never become PUT or current approval", () => page.evaluate(async () => { for (const legacy of [true, false]) { const f = await window.receiptTest.create(), body = f.body(); f.legacyDeclaration = legacy; f.mode = "lost"; await f.actions().checkOriginal(body).catch(() => {}); const original = (await f.actions().rows())[0]; f.mode = "found"; const result = await f.actions().checkOriginal(original.body); if (result.current !== false || f.log.length !== 2 || JSON.stringify(f.log[0].body) !== JSON.stringify(f.log[1].body) || !f.log.every(row => row.route === "/sync/storage-results")) return false; if (!legacy) { let refused = 0; for (const poison of ["partial", "false", "contract", "unknown"]) { f.resultPoison = poison; await f.actions().checkOriginal(original.body).catch(() => refused++); } if (refused !== 4 || !f.log.every(row => row.route === "/sync/storage-results")) return false; } if (JSON.stringify(await f.actions().rows()).includes('"qualified"')) return false; } return true; }));
    await test("same nonce changed declaration and secret fields fail before HTTP", () => page.evaluate(async () => { const f = await window.receiptTest.create(), body = f.body(); await f.actions().checkOriginal(body); let rejected = 0; await f.actions().checkOriginal({ ...body, buildDigest: "d".repeat(64) }).catch(() => rejected++); await f.actions().checkOriginal({ ...body, requestId: crypto.randomUUID(), password: "FORBIDDEN" }).catch(() => rejected++); return rejected === 2 && f.log.length === 1; }));
    await test("late successor family and replaced physical store cannot publish a result", () => page.evaluate(async () => { const t = window.receiptTest; let rejected = 0; const first = await t.create(); first.before = async () => first.install(crypto.randomUUID()); await first.actions().load().catch(() => rejected++); const second = await t.create(); second.before = async () => t.write("sync-state|" + second.identity.namespace, { ...second.state, storeGeneration: crypto.randomUUID() }); await second.actions().load().catch(() => rejected++); return rejected === 2 && first.log.length === 1 && second.log.length === 1; }));
    await test("target preserves exact CV and generation-manifest pair without ACK", () => page.evaluate(async () => { const f = await window.receiptTest.create(), id = crypto.randomUUID(), result = await f.actions().target(id); return result.contentVersion === "9007199254740993" && result.deliveryGeneration === "9007199254740993" && result.manifestDigest === "c".repeat(64) && f.log.length === 1 && f.log[0].method === "GET"; }));
    await test("duplicate JSON aliases cannot replace the original nonce", () => page.evaluate(() => { try { window.receiptTest.json.migrationJSON('{"requestId":"one","request\\u0049d":"two"}'); return false; } catch { return true; } }));
    await test("all ten receipt dictionaries have the same complete nonempty keys", () => page.evaluate(() => { const rows = Object.values(window.receiptTest.labels.receiptLabels), keys = Object.keys(rows[0]).sort().join(); return rows.length === 10 && rows.every(row => Object.keys(row).sort().join() === keys && Object.values(row).every(value => typeof value === "string" && value.length > 0)); }));
    await page.evaluate(async () => { const t = window.receiptTest, f = await t.create(); window.receiptUI = f; t.dom.render(t.React.createElement(t.panel.ReceiptPanel, { context: f.context, enabled: true, locale: "en", t: key => ({ reload: "Reload", loadMore: "Load more", signIn: "Sign in" }[key] ?? key), onLogin: () => {} })); });
    const box = page.locator("#receipt-fixture"); await box.getByText("Server protection ready", { exact: true }).waitFor();
    await test("real React form separates ready server from unverified browser and exposes no ACK or declaration write", async () => (await box.innerText()).includes("This browser: transport only") && await box.locator("button").count() > 0 && !await box.getByRole("button", { name: /acknowledge|approve profile|declare storage/i }).count());
    await box.getByText("Original declaration queries in this browser", { exact: true }).click();
    await box.locator("textarea").fill(await page.evaluate(() => JSON.stringify(window.receiptUI.body())));
    await box.locator("form").first().getByRole("button").click(); await box.getByRole("alert").waitFor();
    await test("real original-result form clears input and preserves query-only responsibility", () => page.evaluate(async () => { const f = window.receiptUI; return document.querySelector("#receipt-fixture textarea").value === "" && (await f.actions().rows())[0].state === "not_found" && f.log.filter(row => row.method === "POST").every(row => row.route === "/sync/storage-results"); }));
    if (pageErrors.length) throw Error("Unhandled page errors: " + pageErrors.join("; "));
    if (JSON.stringify(before) !== JSON.stringify(hashes())) throw Error("Inputs changed during test");
    const snapshots = path.join(out, "输入"); for (const name of sources) { const file = path.join(snapshots, name); fs.mkdirSync(path.dirname(file), { recursive: true }); fs.copyFileSync(path.join(root, name), file); }
    fs.writeFileSync(path.join(out, "结果.json"), JSON.stringify({ startedAt, finishedAt: new Date().toISOString(), passed: checks.length, checks, pageErrors, sourceHashes: before, scope: "Actual Chrome/React/IDB; HTTP/auth fixtures; no qualified browser durability or real HTTPS" }, null, 2));
    console.log(JSON.stringify({ passed: checks.length, out }));
  } catch (error) { fs.writeFileSync(path.join(out, "失败.json"), JSON.stringify({ startedAt, passed: checks.length, checks, error: String(error.stack || error), sourceBefore: before, sourceAfter: hashes() }, null, 2)); throw error; }
  finally { await browser?.close(); await vite.close(); }
})().catch(error => { console.error(error); process.exitCode = 1; });
