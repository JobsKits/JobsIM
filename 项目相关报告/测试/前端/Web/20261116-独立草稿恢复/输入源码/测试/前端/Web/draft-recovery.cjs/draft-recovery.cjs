// Created by Jobs on 2026年11月16日，星期一.
const fs = require("node:fs"), path = require("node:path"), os = require("node:os"), crypto = require("node:crypto"), { pathToFileURL } = require("node:url");
const { chromium } = require(process.env.JOBSIM_PLAYWRIGHT_MODULE || "playwright");
const root = path.resolve(__dirname, "../../../.."), web = path.join(root, "代码/前端/Web");
const out = process.env.JOBSIM_RECOVERY_REPORT || path.join(root, "项目相关报告/测试/前端/Web/执行记录", "draft-recovery-" + Date.now() + "-" + process.pid);
if (fs.existsSync(out)) throw Error("Choose a new report directory");
fs.mkdirSync(out, { recursive: true });
const sources = ["SelfDraftRecovery.tsx", "SelfStateLabels.ts", "SelfStateActions.ts", "SelfStateProtocol.ts", "SyncStore.ts", "App.tsx", "styles.css"].map(name => "代码/前端/Web/Src/" + name).concat("测试/前端/Web/draft-recovery.cjs/draft-recovery.cjs");
const hashes = () => Object.fromEntries(sources.map(file => [file, crypto.createHash("sha256").update(fs.readFileSync(path.join(root, file))).digest("hex")]));
const before = hashes(), checks = [], startedAt = new Date().toISOString();
const assert = (v, message) => { if (!v) throw Error(message); };
(async () => {
  const work = fs.mkdtempSync(path.join(process.env.JOBSIM_RECOVERY_WORK || os.tmpdir(), "jobsim-draft-recovery-"));
  const { createServer } = await import(pathToFileURL(path.join(web, "node_modules/vite/dist/node/index.js")).href);
  // No project proxy/config is loaded, so this server cannot forward an API to a realm.
  const vite = await createServer({ root: web, configFile: false, publicDir: "Public", cacheDir: path.join(work, "Vite"), server: { host: "127.0.0.1", port: 0, strictPort: false } });
  let browser;
  try {
    await vite.listen();
    const url = vite.resolvedUrls.local[0];
    browser = await chromium.launch({ headless: true, executablePath: process.env.BROWSER_EXECUTABLE || "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" });
    const context = await browser.newContext(), page = await context.newPage();
    const apiAttempts = [];
    await page.route("**/api/**", route => { apiAttempts.push(route.request().method() + " " + new URL(route.request().url()).pathname); return route.abort(); });
    await page.goto(url, { waitUntil: "domcontentloaded" });
    await page.evaluate(async () => {
      const store = await import("/Src/SyncStore.ts"), storage = await import("/Src/storage.ts"), types = await import("/Src/types.ts"), actions = await import("/Src/SelfStateActions.ts"), api = await import("/Src/api.ts");
      const module = await import("/Src/SelfDraftRecovery.tsx"), labels = await import("/Src/SelfStateLabels.ts"), i18n = await import("/Src/i18n.ts");
      const resources = performance.getEntriesByType("resource").map(row => row.name), reactURL = resources.find(url => /\/react\.js\?/.test(url)), domURL = resources.find(url => /\/react-dom_client\.js\?/.test(url));
      if (!reactURL || !domURL) throw Error("Real React modules absent");
      const R = await import(reactURL), D = await import(domURL), React = R.default ?? R, dom = D.default ?? D;
      const db = await storage.storageDatabase;
      const read = key => new Promise((resolve, reject) => { const tx = db.transaction("spaces"), request = tx.objectStore("spaces").get(key); tx.oncomplete = () => resolve(request.result); tx.onerror = () => reject(tx.error); });
      const write = rows => new Promise((resolve, reject) => { const tx = db.transaction("spaces", "readwrite", { durability: "strict" }), objects = tx.objectStore("spaces"); for (const [key, value] of rows) objects.put(value, key); tx.oncomplete = resolve; tx.onerror = () => reject(tx.error); });
      const container = document.createElement("div"); container.id = "recovery-fixture"; container.className = "settings-content"; container.style.cssText = "max-width:720px;margin:auto;background:var(--surface);color:var(--text)";
      document.getElementById("root").style.display = "none"; document.body.append(container);
      const domRoot = dom.createRoot(container), copies = [];
      Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText: async text => { copies.push(text); } } });
      const create = async () => {
        const f = { scope: crypto.randomUUID(), user: crypto.randomUUID(), device: crypto.randomUUID(), cid: crypto.randomUUID(), draftId: crypto.randomUUID(), originalCID: crypto.randomUUID(), activeCID: crypto.randomUUID(), current: true, enabled: true, log: [], errors: [], changed: 0, locale: "en", hold: false, failGET: false, proofs: {} };
        f.identity = { scopeId: f.scope, principalId: f.user, deviceId: f.device, sourceContextId: "personal", restoreEpoch: "2" };
        f.namespace = location.origin + "|" + f.scope + "|" + f.user + "|personal|2|" + f.device;
        f.base = cid => ({ id: cid === f.cid ? f.draftId : crypto.randomUUID(), scopeId: f.scope, userId: f.user, sourceContextId: "personal", revisionEpoch: "2", conversationId: cid, version: "9007199254740993", contentState: "owned", ownedText: "CURRENT_ALLOWED_TEXT", replyState: "unavailable", reply: null, writable: false });
        f.live = { [f.cid]: { ...f.base(f.cid), draftAdmission: "da1.FIXTURE_CLEAR_ONLY", recovery: { operationId: crypto.randomUUID(), ownedText: "SERVER_RECOVERY_ONLY" } }, [f.originalCID]: { ...f.base(f.originalCID), ownedText: "RETIRED_GET_ALLOWED_TEXT", draftAdmission: "da1.OTHER_CLEAR" } };
        f.personal = { records: { ["draft|" + f.draftId]: { ...f.base(f.cid), ownedText: "NEVER_RENDER_METADATA_BODY" } } };
        await write([["auth-active|" + f.namespace.split("|").slice(0, 4).join("|"), f.namespace], [f.namespace, { ...types.emptySpace(), drafts: { [f.activeCID]: "UNTOUCHED_OTHER_INPUT" }, conversations: [{ id: f.activeCID }] }]]);
        const state = await store.ensureSyncState(f.namespace, f.identity, () => f.current);
        f.state = { ...state, protocolSchema: "2", restriction: undefined, streamId: crypto.randomUUID(), cursor: "PRIVATE_FIXTURE" };
        const body = JSON.stringify({ requestId: crypto.randomUUID(), revisionEpoch: "2", expectedVersion: "1", draftAdmission: "da1.ORIGINAL", ownedText: "NEVER_RENDER_ORIGINAL_BODY", reply: { messageId: crypto.randomUUID(), sourceVersion: "1", replyAdmission: "dr1.NEVER_RENDER_PROOF" } });
        const parsed = JSON.parse(body), digest = [...new Uint8Array(await crypto.subtle.digest("SHA-256", new TextEncoder().encode(body)))].map(v => v.toString(16).padStart(2, "0")).join("");
        f.original = { requestId: parsed.requestId, kind: "draft", channel: "draft|" + f.originalCID, path: "/conversations/" + f.originalCID + "/draft", method: "PUT", originalBody: body, bodySHA256: digest, storeGeneration: state.storeGeneration, guardGeneration: state.guardGeneration, status: "retired" };
        f.operations = [f.original];
        await write([["sync-state|" + f.namespace, f.state], ["sync-personal|" + f.namespace, f.personal], ["self-state-operations|" + f.namespace, f.operations]]);
        f.request = async (path, init = {}) => {
          const method = init.method || "GET", body = init.body ? JSON.parse(init.body) : undefined;
          f.log.push({ path, method, raw: init.body });
          if (path === "/bootstrap") return { deploymentId: f.scope, features: { sync_self_state: f.enabled, cloud_drafts: f.enabled } };
          const match = path.match(/^\/conversations\/([a-f0-9-]+)\/draft(-results)?$/);
          if (!match) throw Error("Unexpected source/history route");
          const cid = match[1];
          if (match[2]) {
            const result = f.proofs[body.body.requestId];
            if (!result) throw new api.ApiError(404, "DRAFT_OPERATION_NOT_FOUND", "injected original missing");
            return structuredClone(result);
          }
          if (method === "GET") {
            if (f.hold) { f.hold = false; await new Promise(resolve => { f.releaseGET = resolve; }); }
            if (f.failGET) throw new api.ApiError(503, "INJECTED_UNKNOWN", "injected current GET unknown");
            f.deliveredGET = (f.deliveredGET || 0) + 1;
            return structuredClone(f.live[cid]);
          }
          if (method !== "DELETE") throw Error("Recovery tried to create/update a draft");
          if (body.expectedVersion !== f.live[cid].version) throw new api.ApiError(409, "DRAFT_VERSION_CONFLICT", "injected source CAS");
          const result = { operationId: crypto.randomUUID(), status: "applied", current: true, result: { revisionEpoch: "2", version: String(BigInt(f.live[cid].version) + 1n), contentState: "empty", replyState: "none" } };
          f.proofs[body.requestId] = result;
          f.live[cid] = { ...f.live[cid], version: result.result.version, contentState: "empty", ownedText: null, replyState: "none", reply: null };
          return structuredClone(result);
        };
        f.actions = new actions.SelfStateActions({ namespace: f.namespace, identity: f.identity, isCurrent: () => f.current, request: f.request });
        f.render = () => domRoot.render(React.createElement(module.default, { key: f.namespace, actions: f.actions, conversationIds: [f.activeCID], locale: f.locale, t: key => i18n.translate(f.locale, key), onChanged: () => f.changed++, onError: error => f.errors.push(error.code || error.message) }));
        return f;
      };
      window.recoveryTest = { store, read, write, create, candidates: module.draftRecoveryCandidates, labels: labels.selfStateLabels, copies, domRoot, React, module };
    });
    const mount = async () => { await page.evaluate(async () => { window.recoveryFixture = await window.recoveryTest.create(); window.recoveryFixture.render(); }); await page.getByRole("button", { name: /Former conversation draft/ }).first().waitFor(); };
    const open = async which => { const suffix = await page.evaluate(key => window.recoveryFixture[key].slice(-8), which); await page.getByRole("button", { name: new RegExp(suffix) }).click(); await page.getByText(which === "cid" ? "CURRENT_ALLOWED_TEXT" : "RETIRED_GET_ALLOWED_TEXT", { exact: true }).waitFor(); };
    await mount();
    assert(await page.evaluate(() => {
      const t = window.recoveryTest, f = window.recoveryFixture, wrong = { ...f.original, kind: "view" }, malformed = { ...f.original, channel: "draft|not-a-uuid" }, foreign = { records: { ["draft|" + f.draftId]: { ...f.personal.records["draft|" + f.draftId], userId: crypto.randomUUID() } } };
      return t.candidates(foreign, [wrong, malformed], [], f.identity).length === 0 && t.candidates(f.personal, [], [f.cid], f.identity).length === 1 && t.candidates({ records: { ["draft|" + f.draftId]: { ...f.personal.records["draft|" + f.draftId], writable: true } } }, [], [f.cid], f.identity).length === 0;
    }), "candidate identity/channel rules failed");
    checks.push("private candidates validate owner/epoch/kind/channel/UUID; readonly or absent conversation and retired-only intents are discoverable");
    assert(!(await page.locator("body").innerText()).match(/NEVER_RENDER|CURRENT_ALLOWED_TEXT|SERVER_RECOVERY_ONLY/), "metadata or original intent appeared before exact GET");
    await open("originalCID");
    assert(!JSON.stringify(await page.evaluate(() => window.recoveryFixture.log)).includes("/messages/"), "source body route used");
    checks.push("retired-only candidate without a personal projection opens exact authorized draft GET without fabricating a conversation or reading source");
    const original = await page.evaluate(() => JSON.stringify(window.recoveryFixture.original));
    await page.getByRole("button", { name: /Only check original action ·/ }).click();
    await page.waitForFunction(() => window.recoveryFixture.errors.includes("DRAFT_OPERATION_NOT_FOUND"));
    assert(await page.evaluate(async old => {
      const f = window.recoveryFixture, stored = (await window.recoveryTest.read("self-state-operations|" + f.namespace)).find(row => row.requestId === f.original.requestId), expected = JSON.parse(old);
      return ["requestId", "originalBody", "bodySHA256", "path", "method", "storeGeneration"].every(key => stored[key] === expected[key]) && !f.log.some(row => row.method === "PUT" || row.method === "DELETE");
    }, original), "unknown original was rewritten/created");
    checks.push("unknown original result uses only original-body draft-results, with zero PUT/DELETE or proof/nonce replacement");
    await open("cid");
    assert(await page.getByText("SERVER_RECOVERY_ONLY", { exact: true }).isVisible(), "GET recovery missing");
    assert(!(await page.locator("body").innerText()).includes("NEVER_RENDER"), "old body/proof leaked");
    await page.getByRole("button", { name: "Copy text", exact: true }).click();
    await page.waitForFunction(() => window.recoveryTest.copies.includes("CURRENT_ALLOWED_TEXT"));
    await page.getByRole("button", { name: "Copy text · Recoverable text", exact: true }).click();
    await page.waitForFunction(() => window.recoveryTest.copies.includes("SERVER_RECOVERY_ONLY"));
    checks.push("explicit copy revalidates the displayed version/text and copies only current GET ownedText or current GET recovery text into an injected clipboard sink");
    await page.evaluate(() => { const f = window.recoveryFixture; f.live[f.cid].version = "9007199254740994"; f.live[f.cid].ownedText = "NEW_REMOTE_TEXT"; });
    const copies = await page.evaluate(() => window.recoveryTest.copies.length);
    await page.getByRole("button", { name: "Copy text", exact: true }).click();
    await page.waitForFunction(() => window.recoveryFixture.errors.includes("SELF_STATE_VERSION_CHANGED"));
    assert(await page.evaluate(n => window.recoveryTest.copies.length === n, copies), "stale selection copied a different remote version");
    checks.push("late remote text/version change does not silently change the user-selected copy");
    const localBefore = await page.evaluate(() => window.recoveryTest.read(window.recoveryFixture.namespace));
    await page.getByRole("button", { name: "Clear cloud draft", exact: true }).click();
    await page.evaluate(() => { const f = window.recoveryFixture; f.live[f.cid].version = "9007199254740995"; f.live[f.cid].ownedText = "LATEST_REMOTE_TEXT"; });
    await page.getByRole("button", { name: "Confirm clear", exact: true }).click();
    await page.waitForFunction(() => window.recoveryFixture.errors.filter(x => x === "SELF_STATE_VERSION_CHANGED").length === 2);
    assert(await page.evaluate(() => !window.recoveryFixture.log.some(row => row.method === "DELETE")), "stale clear replaced the shown CAS");
    await page.getByRole("button", { name: "Reload", exact: true }).first().click();
    await page.getByText("LATEST_REMOTE_TEXT", { exact: true }).waitFor();
    await page.getByRole("button", { name: "Clear cloud draft", exact: true }).click();
    await page.getByRole("button", { name: "Confirm clear", exact: true }).click();
    await page.waitForFunction(() => window.recoveryFixture.changed === 1);
    assert(await page.evaluate(async before => JSON.stringify(await window.recoveryTest.read(window.recoveryFixture.namespace)) === JSON.stringify(before) && window.recoveryFixture.log.filter(row => row.method === "DELETE").length === 1 && !window.recoveryFixture.log.some(row => row.method === "PUT"), localBefore), "clear touched another input or replayed PUT");
    checks.push("confirmed clear captures the shown exact large version, refuses stale CAS, and preserves another conversation's entire local input/Space");
    await mount(); await open("cid");
    await page.evaluate(() => { const f = window.recoveryFixture; f.failGET = true; });
    await page.getByRole("button", { name: "Reload", exact: true }).first().click();
    await page.waitForFunction(() => window.recoveryFixture.errors.includes("INJECTED_UNKNOWN"));
    assert(!(await page.locator("body").innerText()).includes("CURRENT_ALLOWED_TEXT") && !(await page.locator("body").innerText()).includes("NEVER_RENDER"), "failed current GET fell back to old metadata/original body");
    await page.evaluate(() => { window.recoveryFixture.failGET = false; window.recoveryFixture.enabled = false; });
    await page.getByRole("button", { name: "Reload", exact: true }).first().click();
    await page.waitForFunction(() => window.recoveryFixture.errors.includes("SYNC_SELF_STATE_UNAVAILABLE"));
    assert(await page.evaluate(() => !window.recoveryFixture.log.some(row => row.method === "PUT" || row.method === "DELETE")), "capability withdrawal wrote an intent");
    checks.push("unknown GET and capability withdrawal show reload/error without restoring text from private metadata or original bodies");
    await mount(); await open("cid");
    await page.evaluate(async () => { const f = window.recoveryFixture; window.retiredFamily = f; f.hold = true; });
    await page.getByRole("button", { name: "Reload", exact: true }).first().click();
    await page.waitForFunction(() => typeof window.retiredFamily.releaseGET === "function");
    const delivered = await page.evaluate(() => window.retiredFamily.deliveredGET);
    await page.evaluate(async () => { const old = window.retiredFamily; old.current = false; const next = await window.recoveryTest.create(); next.personal = { records: {} }; next.operations = []; await window.recoveryTest.write([["sync-personal|" + next.namespace, next.personal], ["self-state-operations|" + next.namespace, []]]); window.recoveryFixture = next; next.render(); old.releaseGET(); });
    await page.waitForFunction(n => window.retiredFamily.deliveredGET > n, delivered);
    await page.getByText("No independent drafts to review", { exact: true }).waitFor();
    assert(!(await page.locator("body").innerText()).includes("CURRENT_ALLOWED_TEXT"), "old family late GET leaked");
    await page.getByRole("button", { name: "Reload", exact: true }).first().click();
    await page.getByText("No independent drafts to review", { exact: true }).waitFor();
    checks.push("a late exact GET from the previous family/namespace cannot publish into the remounted empty current context");
    await mount(); await open("cid");
    await page.evaluate(() => { const f = window.recoveryFixture; navigator.locks.request("jobsim-self-state|" + f.namespace, () => new Promise(resolve => { f.releaseLock = resolve; })); });
    await page.waitForFunction(() => typeof window.recoveryFixture.releaseLock === "function");
    await page.getByRole("button", { name: "Clear cloud draft", exact: true }).click();
    await page.getByRole("button", { name: "Confirm clear", exact: true }).click();
    await page.evaluate(async () => { const f = window.recoveryFixture; f.current = false; f.releaseLock(); await navigator.locks.request("jobsim-self-state|" + f.namespace, () => {}); });
    assert(await page.evaluate(() => !window.recoveryFixture.log.some(row => row.method === "PUT" || row.method === "DELETE")), "queued old-family clear escaped its context guard");
    checks.push("an actual Web Locks queued clear is cancelled after family invalidation before any creating write");
    await mount(); await open("cid"); await page.setViewportSize({ width: 390, height: 844 });
    const locales = await page.evaluate(() => Object.keys(window.recoveryTest.labels));
    for (const locale of locales) for (const theme of ["light", "dark", "system"]) {
      await page.evaluate(({ locale, theme }) => { const f = window.recoveryFixture; f.locale = locale; document.documentElement.lang = locale; document.documentElement.dataset.theme = theme === "system" ? (matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light") : theme; f.render(); }, { locale, theme });
      await page.waitForTimeout(20);
      assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), "mobile overflow " + locale + "/" + theme);
      assert(await page.evaluate(() => { const selected = document.querySelector('#recovery-fixture button[aria-pressed="true"]'); return selected && getComputedStyle(selected).borderInlineStartColor !== "rgba(0, 0, 0, 0)"; }), "selection cue missing " + locale + "/" + theme);
      assert(await page.evaluate(locale => { const r = window.recoveryTest.labels[locale]; return Object.keys(r).length === Object.keys(window.recoveryTest.labels.en).length && Object.values(r).every(v => typeof v === "string" && v.trim()); }, locale), "translation coverage " + locale);
    }
    for (const [locale, theme] of [["zh-CN", "light"], ["en", "dark"], ["bo", "system"]]) {
      await page.evaluate(({ locale, theme }) => { const f = window.recoveryFixture; f.locale = locale; document.documentElement.lang = locale; document.documentElement.dataset.theme = theme === "system" ? "light" : theme; f.render(); }, { locale, theme });
      await page.waitForTimeout(40); await page.screenshot({ path: path.join(out, "Recovery-" + locale + "-" + theme + ".png"), fullPage: true });
    }
    checks.push("ten languages inherit light/dark/resolved-system tokens at mobile width without horizontal overflow; three current-text screenshots retained");
    assert(JSON.stringify(before) === JSON.stringify(hashes()), "source changed during browser run");
    fs.writeFileSync(path.join(out, "结果.json"), JSON.stringify({ startedAt, finishedAt: new Date().toISOString(), status: "passed", checks, sourceSHA256: before, sourceUnchanged: true, scope: "Real Chrome/React/IndexedDB/Web Locks; injected draft request function and clipboard sink. Private Vite has no API proxy; all browser API routes aborted. No E2/live server/device storage attestation.", blockedAPIAttempts: apiAttempts, work }, null, 2));
    process.stdout.write(JSON.stringify({ status: "passed", groups: checks.length, report: out }) + "\n");
  } catch (error) {
    fs.writeFileSync(path.join(out, "结果.json"), JSON.stringify({ startedAt, status: "failed", error: String(error.stack || error), checks, sourceSHA256: before, scope: "injected local browser only; no live service" }, null, 2));
    throw error;
  } finally { if (browser) await browser.close(); await vite.close(); }
})().catch(error => { process.stderr.write(String(error.stack || error) + "\n"); process.exitCode = 1; });
