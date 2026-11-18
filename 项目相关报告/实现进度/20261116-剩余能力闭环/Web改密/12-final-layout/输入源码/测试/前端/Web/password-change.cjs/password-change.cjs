// Created by Jobs on 2026年11月16日，星期一.
// Real browser/IndexedDB/Web Locks; all server responses and passwords below are isolated fixtures.
const fs = require("node:fs"), path = require("node:path"), os = require("node:os"), crypto = require("node:crypto"), { pathToFileURL } = require("node:url");
const { chromium } = require(process.env.JOBSIM_PLAYWRIGHT_MODULE || "playwright");
const root = path.resolve(__dirname, "../../../.."), web = path.join(root, "代码/前端/Web");
const out = process.env.JOBSIM_PASSWORD_REPORT || path.join(root, "项目相关报告/测试/前端/Web/执行记录", "password-change-" + Date.now() + "-" + process.pid);
const free = () => { const s = fs.statfsSync(root); return Number(s.bavail) * Number(s.bsize); };
if (free() < 4 * 1024 ** 3) throw Error("4GiB disk gate");
if (fs.existsSync(out)) throw Error("Choose a new report directory");
fs.mkdirSync(out, { recursive: true });
const sources = fs.readdirSync(path.join(web, "Src")).filter(name => /\.(ts|tsx|css)$/.test(name)).map(name => "代码/前端/Web/Src/" + name).concat(path.relative(root, __filename));
const hashes = () => Object.fromEntries(sources.map(file => [file, crypto.createHash("sha256").update(fs.readFileSync(path.join(root, file))).digest("hex")]));
const before = hashes(), checks = [], startedAt = new Date().toISOString();
const assert = (value, reason) => { if (!value) throw Error(reason); };
(async () => {
  const work = fs.mkdtempSync(path.join(os.tmpdir(), "JobsIM-PasswordChange-"));
  const { createServer } = await import(pathToFileURL(path.join(web, "node_modules/vite/dist/node/index.js")).href);
  const vite = await createServer({ root: web, configFile: false, publicDir: "Public", cacheDir: path.join(work, "Vite"), server: { host: "127.0.0.1", port: 0 } });
  let browser;
  try {
    await vite.listen();
    browser = await chromium.launch({ headless: true, executablePath: process.env.BROWSER_EXECUTABLE || "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" });
    const context = await browser.newContext(), page = await context.newPage(), apiAttempts = [];
    await page.route("**/api/**", route => { apiAttempts.push(route.request().method() + " " + new URL(route.request().url()).pathname); return route.abort(); });
    await page.goto(vite.resolvedUrls.local[0], { waitUntil: "domcontentloaded" });
    await page.evaluate(async () => {
      const actions = await import("/Src/PasswordChangeActions.ts"), store = await import("/Src/PasswordChangeStore.ts"), protocol = await import("/Src/PasswordChangeProtocol.ts"), auth = await import("/Src/AuthSession.ts"), api = await import("/Src/api.ts"), database = await import("/Src/StorageDatabase.ts"), labels = await import("/Src/PasswordChangeLabels.ts"), panel = await import("/Src/PasswordChangePanel.tsx"), i18n = await import("/Src/i18n.ts");
      const resources = performance.getEntriesByType("resource").map(r => r.name), reactURL = resources.find(url => /\/react\.js\?/.test(url)), domURL = resources.find(url => /\/react-dom_client\.js\?/.test(url));
      if (!reactURL || !domURL) throw Error("Real React modules absent");
      const R = await import(reactURL), D = await import(domURL), React = R.default ?? R, dom = D.default ?? D;
      const db = await database.storageDatabase;
      const read = key => new Promise((resolve, reject) => { const tx = db.transaction("spaces"), q = tx.objectStore("spaces").get(key); tx.oncomplete = () => resolve(q.result); tx.onerror = () => reject(tx.error); });
      const write = (key, value) => new Promise((resolve, reject) => { const tx = db.transaction("spaces", "readwrite", { durability: "strict" }); tx.objectStore("spaces").put(value, key); tx.oncomplete = resolve; tx.onerror = () => reject(tx.error); });
      const container = document.createElement("div"); container.className = "settings-content"; container.id = "password-fixture";
      document.getElementById("root").style.display = "none"; document.body.append(container); const domRoot = dom.createRoot(container);
      const create = () => {
        const f = { current: true, feature: true, mode: "changed", log: [], saved: {}, pauses: 0, locale: "en", administrator: false, logins: 0 };
        f.identity = { scopeId: crypto.randomUUID(), userId: crypto.randomUUID(), username: "password_fixture", deviceId: crypto.randomUUID(), epoch: "3", familyId: crypto.randomUUID() };
        f.identity.namespace = [location.origin, f.identity.scopeId, f.identity.userId, "personal", f.identity.epoch, f.identity.deviceId].join("|");
        f.proof = { currentPassword: "Original-password-123", newPassword: "Replacement-password-456", otp: "123456" };
        f.install = family => {
          auth.configureAuthProtocol(true, f.identity.scopeId, true);
          const value = { familyId: family ?? f.identity.familyId, refreshGeneration: "0", accessExpiresAt: new Date(Date.now() + 100000).toISOString(), idleExpiresAt: new Date(Date.now() + 200000).toISOString(), absoluteExpiresAt: new Date(Date.now() + 300000).toISOString(), sourceContextId: "personal", restoreEpoch: f.identity.epoch, deviceId: f.identity.deviceId, user: { id: f.identity.userId, username: f.identity.username } };
          auth.rememberPublicAuth(auth.parsePublicAuth(value));
        };
        f.install();
        f.result = (id, status = "changed", current = true) => ({ requestId: id, status, restoreEpoch: f.identity.epoch, credentialGeneration: "9007199254740993", securityVersion: "9007199254740994", current: status === "changed" && current, requiresNewLogin: status === "changed" });
        f.fetch = async (url, options) => {
          const route = new URL(url, location.origin).pathname, headers = new Headers(options.headers), body = options.body ? JSON.parse(options.body) : undefined;
          f.log.push({ route, method: options.method, credentials: options.credentials, family: headers.get("X-JobsIM-Auth-Family"), csrf: headers.get("X-CSRF-Token"), body });
          if (headers.get("X-JobsIM-Client") !== "web" || options.redirect !== "error") throw Error("Missing browser/redirect protection");
          if (route === "/api/bootstrap") return Response.json({ deploymentId: f.identity.scopeId, restoreEpoch: f.identity.epoch, features: { own_password_change: f.feature } });
          if (route === "/api/auth/csrf") { if (f.beforeCSRF) f.beforeCSRF(); return Response.json({ csrfToken: "fixture-csrf" }); }
          if (route === "/api/auth/password-change-results") {
            if (options.credentials !== "omit" || headers.has("X-JobsIM-Auth-Family") || headers.has("X-CSRF-Token") || "otp" in body) throw Error("Original lookup carried current credentials/OTP");
            if (f.holdLookup) { f.holdLookup = false; await new Promise(resolve => { f.releaseLookup = resolve; }); }
            if (f.badLookup) return Response.json({ ...f.result(body.requestId), current: "true" });
            const existing = f.saved[body.requestId];
            if (existing && (existing.currentPassword !== body.currentPassword || existing.newPassword !== body.newPassword)) return Response.json({ error: { code: "PASSWORD_CHANGE_REJECTED" } }, { status: 403 });
            return existing ? Response.json(existing.result) : Response.json({ error: { code: "PASSWORD_CHANGE_OPERATION_NOT_FOUND" } }, { status: 404 });
          }
          if (route !== "/api/auth/password-change") throw Error("Unexpected request: " + route);
          const originals = await store.passwordChangeIntents(f.context), original = originals.find(row => row.requestId === body.requestId);
          if (!original || original.attempts < 1 || original.state !== "unknown") throw Error("HTTP preceded durable responsibility");
          if (!f.saved[body.requestId] && (options.credentials !== "include" || headers.get("X-JobsIM-Auth-Family") !== f.identity.familyId || headers.get("X-CSRF-Token") !== "fixture-csrf")) throw Error("Fresh change lacks original family/CSRF");
          if (f.mode === "drop-before") { f.mode = "changed"; throw Error("injected pre-delivery loss"); }
          if (f.mode === "rejected") return Response.json({ error: { code: "PASSWORD_CHANGE_REJECTED" } }, { status: 403 });
          const result = f.result(body.requestId, f.mode === "pending" ? "pending" : "changed");
          f.saved[body.requestId] = { ...body, result };
          if (f.mode === "drop-after") { f.mode = "changed"; throw Error("injected committed lost response"); }
          if (f.mode === "late-family") { f.install(crypto.randomUUID()); return Response.json(result); }
          if (f.mode === "bad200") return Response.json({ ...result, restoreEpoch: "99" });
          return Response.json(result);
        };
        f.context = { scopeId: f.identity.scopeId, identity: f.identity, isCurrent: () => f.current, fetch: f.fetch, onPause: () => f.pauses++ };
        f.actions = () => new actions.PasswordChangeActions(f.context);
        f.render = () => domRoot.render(React.createElement(panel.default, { key: f.identity.scopeId, context: f.context, enabled: f.feature, administrator: f.administrator, locale: f.locale, t: key => i18n.translate(f.locale, key), onLogin: () => f.logins++ }));
        return f;
      };
      window.passwordTest = { actions, store, protocol, auth, api, labels, panel, i18n, read, write, React, domRoot, create, container, db };
    });
    const test = async (name, execute) => { assert(await execute(), name); checks.push(name); };
    await test("original non-secret responsibility commits before HTTP; success retires only its original family", () => page.evaluate(async () => {
      const f = window.passwordTest.create(), r = await f.actions().create(f.proof), rows = await f.actions().rows(), raw = JSON.stringify(rows);
      return r.state === "changed" && !window.passwordTest.auth.authSnapshot().auth && f.pauses === 1 && ![f.proof.currentPassword, f.proof.newPassword, "currentPassword", "newPassword", "otp", "digest", "hash"].some(secret => raw.includes(secret)) && f.log.filter(r => r.route.endsWith("password-change")).length === 1;
    }));
    await test("committed lost response is recovered with original nonce and anonymous result lookup, never another write", () => page.evaluate(async () => {
      const f = window.passwordTest.create(); f.mode = "drop-after"; await f.actions().create(f.proof).catch(() => {}); const original = (await f.actions().rows())[0], count = f.log.length;
      const result = await f.actions().checkOriginal(original.requestId, f.proof);
      return original.state === "unknown" && result.state === "changed" && f.log.slice(count).every(row => !row.route.endsWith("/password-change")) && result.requestId === original.requestId;
    }));
    await test("pre-delivery loss plus automatic original 404 creates nothing; explicit same-family retry keeps the nonce", () => page.evaluate(async () => {
      const f = window.passwordTest.create(); f.mode = "drop-before"; await f.actions().create(f.proof).catch(() => {}); const row = (await f.actions().rows())[0], writes = () => f.log.filter(r => r.route.endsWith("/password-change"));
      await f.actions().checkOriginal(row.requestId, f.proof); if (writes().length !== 1) return false;
      const result = await f.actions().checkOriginal(row.requestId, f.proof, true);
      return result.state === "changed" && writes().length === 2 && writes().every(r => r.body.requestId === row.requestId);
    }));
    await test("successor family cannot authorize an unseen old intent or be cleared by an old completed result", () => page.evaluate(async () => {
      const f = window.passwordTest.create(); f.mode = "drop-before"; await f.actions().create(f.proof).catch(() => {}); const row = (await f.actions().rows())[0], next = crypto.randomUUID(); f.install(next);
      await f.actions().checkOriginal(row.requestId, f.proof, true).catch(() => {});
      if (f.log.filter(r => r.route.endsWith("/password-change")).length !== 1) return false;
      f.saved[row.requestId] = { ...f.proof, result: f.result(row.requestId, "changed", false) };
      await f.actions().checkOriginal(row.requestId, f.proof);
      return window.passwordTest.auth.authSnapshot().auth.familyId === next && !(await window.passwordTest.read("auth-frozen|" + row.namespace));
    }));
    await test("cold original metadata remains usable without a session, while prepared continuation stays anonymous", () => page.evaluate(async () => {
      const f = window.passwordTest.create(); f.mode = "pending"; await f.actions().create(f.proof); const row = (await f.actions().rows())[0]; window.passwordTest.auth.clearPublicAuth(); delete f.context.identity;
      f.mode = "changed"; const result = await new window.passwordTest.actions.PasswordChangeActions(f.context).checkOriginal(row.requestId, f.proof, true);
      const writes = f.log.filter(r => r.route.endsWith("/password-change")); return result.state === "changed" && writes.length === 2 && writes[1].credentials === "omit" && !writes[1].family && !writes[1].csrf;
    }));
    await test("capability withdrawal permits original result checks but refuses unseen publication", () => page.evaluate(async () => {
      const f = window.passwordTest.create(); f.mode = "drop-before"; await f.actions().create(f.proof).catch(() => {}); const row = (await f.actions().rows())[0]; f.feature = false;
      await f.actions().checkOriginal(row.requestId, f.proof); await f.actions().checkOriginal(row.requestId, f.proof, true).catch(() => {});
      return f.log.filter(r => r.route.endsWith("/password-change")).length === 1;
    }));
    await test("OTP wait/capability checks reject a changed original family before authorized HTTP", () => page.evaluate(async () => {
      const f = window.passwordTest.create(); f.beforeCSRF = () => f.install(crypto.randomUUID()); await f.actions().create(f.proof).catch(() => {});
      return f.log.filter(r => r.route.endsWith("/password-change")).length === 0 && (await f.actions().rows())[0].attempts === 0;
    }));
    await test("malformed success and late-family results never confirm or clear replacement credentials", () => page.evaluate(async () => {
      const f = window.passwordTest.create(); f.mode = "bad200"; await f.actions().create(f.proof).catch(() => {}); if ((await f.actions().rows())[0].state !== "unknown") return false;
      const g = window.passwordTest.create(); g.mode = "late-family"; await g.actions().create(g.proof).catch(() => {});
      return (await g.actions().rows())[0].state === "unknown" && window.passwordTest.auth.authSnapshot().auth.familyId !== g.identity.familyId;
    }));
    await test("exact first rejection keeps a rejected original and never invokes automatic refresh", () => page.evaluate(async () => {
      const f = window.passwordTest.create(); f.mode = "rejected"; await f.actions().create(f.proof).catch(() => {});
      return (await f.actions().rows())[0].state === "rejected" && f.pauses === 0 && !f.log.some(r => r.route.includes("refresh"));
    }));
    await test("unknown responsibility gates ordinary APIs and survives a reopened action service", () => page.evaluate(async () => {
      const f = window.passwordTest.create(); f.mode = "drop-before"; await f.actions().create(f.proof).catch(() => {});
      let code; try { await window.passwordTest.api.api("/users"); } catch (error) { code = error.code; }
      const rows = await new window.passwordTest.actions.PasswordChangeActions(f.context).rows();
      return code === "PASSWORD_CHANGE_RESULT_UNKNOWN" && rows.length === 1 && rows[0].attempts === 1;
    }));
    await test("strict IndexedDB abort precedes any first publication and forbids leaked password metadata", () => page.evaluate(async () => {
      const f = window.passwordTest.create(), native = IDBObjectStore.prototype.put; let failed = false;
      IDBObjectStore.prototype.put = function(value, key) { if (typeof key === "string" && key.startsWith("password-changes|")) { this.transaction.abort(); failed = true; return; } return native.call(this, value, key); };
      try { await f.actions().create(f.proof).catch(() => {}); } finally { IDBObjectStore.prototype.put = native; }
      return failed && !f.log.some(r => r.route.endsWith("/password-change"));
    }));
    await test("same-operation wrong password proof only reads and does not invent a replacement intent", () => page.evaluate(async () => {
      const f = window.passwordTest.create(); f.mode = "pending"; await f.actions().create(f.proof); const row = (await f.actions().rows())[0];
      await f.actions().checkOriginal(row.requestId, { ...f.proof, newPassword: "Different-password-789" }, true).catch(() => {});
      return (await f.actions().rows()).length === 1 && f.log.filter(r => r.route.endsWith("/password-change")).length === 1;
    }));
    await test("real lifecycle lock cancels an old queued action after family replacement without any HTTP", () => page.evaluate(async () => {
      const f = window.passwordTest.create(); let entered, release;
      const ready = new Promise(resolve => { entered = resolve; });
      const held = navigator.locks.request("jobsim-auth-lifecycle", async () => { entered(); await new Promise(resolve => { release = resolve; }); });
      await ready; const action = f.actions().create(f.proof).then(() => false, () => true); f.install(crypto.randomUUID()); release(); await held;
      return await action && f.log.length === 0 && (await f.actions().rows()).length === 0;
    }));
    await test("NFC proof handling preserves significant spaces and never persists proof material", () => page.evaluate(async () => {
      const f = window.passwordTest.create(); await f.actions().create({ currentPassword: " Original-password-123 ", newPassword: "e\u0301".repeat(15) });
      const body = f.log.find(row => row.route.endsWith("/password-change")).body, raw = JSON.stringify(await f.actions().rows());
      return body.currentPassword === " Original-password-123 " && body.newPassword === "é".repeat(15) && !raw.includes("currentPassword") && !raw.includes("newPassword") && !raw.includes("é");
    }));
    await test("unexpected fields supplied with identity cannot leak into durable metadata", () => page.evaluate(async () => {
      const f = window.passwordTest.create(); f.context.identity = { ...f.identity, currentPassword: "MUST_NOT_PERSIST", otp: "654321", bodyHash: "UNKEYED_HASH_NOT_ALLOWED" };
      await f.actions().create(f.proof); const raw = JSON.stringify(await f.actions().rows());
      return !["MUST_NOT_PERSIST", "654321", "bodyHash", "UNKEYED_HASH_NOT_ALLOWED"].some(value => raw.includes(value));
    }));
    await test("an IndexedDB gate wait cannot borrow a successor family for an older ordinary API request", () => page.evaluate(async () => {
      const f = window.passwordTest.create(), native = window.fetch; let calls = 0, hold = true, entered;
      window.fetch = async () => { calls++; return Response.json({ items: [] }); };
      const ready = new Promise(resolve => { entered = resolve; }), tx = window.passwordTest.db.transaction("spaces", "readwrite"), objects = tx.objectStore("spaces");
      const loop = () => { const q = objects.get("password-gate-test"); q.onsuccess = () => { entered(); if (hold) loop(); }; }; loop();
      try {
        await ready; const request = window.passwordTest.api.api("/users").then(() => false, error => error.code === "AUTH_REQUEST_ABORTED");
        await Promise.resolve(); await Promise.resolve(); f.install(crypto.randomUUID()); hold = false;
        return await request && calls === 0;
      } finally { hold = false; window.fetch = native; }
    }));
    await test("cold BFF context learning cannot reactivate the family with durable unknown responsibility", async () => {
      const metadata = await page.evaluate(async () => {
        const f = window.passwordTest.create(); f.mode = "drop-before"; await f.actions().create(f.proof).catch(() => {}); window.passwordTest.auth.clearPublicAuth();
        return { familyId: f.identity.familyId, refreshGeneration: "0", accessExpiresAt: new Date(Date.now() + 100000).toISOString(), idleExpiresAt: new Date(Date.now() + 200000).toISOString(), absoluteExpiresAt: new Date(Date.now() + 300000).toISOString(), restoreEpoch: f.identity.epoch, sourceContextId: "personal", deviceId: f.identity.deviceId, user: { id: f.identity.userId, username: f.identity.username } };
      });
      const handler = route => route.fulfill({ json: metadata });
      await page.route("**/api/auth/context", handler);
      try { return await page.evaluate(async () => { let code; try { await window.passwordTest.api.api("/auth/context"); } catch (error) { code = error.code; } return code === "PASSWORD_CHANGE_RESULT_UNKNOWN" && window.passwordTest.auth.authSnapshot().auth === null; }); }
      finally { await page.unroute("**/api/auth/context", handler); }
    });
    await test("actual anonymous fetch ignores response Set-Cookie and never sends the successor browser Cookie", async () => {
      const fixture = await page.evaluate(async () => {
        const f = window.passwordTest.create(); f.mode = "pending"; await f.actions().create(f.proof); const row = (await f.actions().rows())[0], next = crypto.randomUUID(); f.install(next); delete f.context.fetch; window.cookiePasswordFixture = f;
        return { origin: location.origin, row, result: f.result(row.requestId, "changed", false), next };
      });
      await context.addCookies([{ name: "jobsim_password_fixture", value: "successor", url: fixture.origin }]);
      let cookieSent = false;
      const bootstrap = route => route.fulfill({ json: { deploymentId: fixture.row.scopeId, restoreEpoch: fixture.row.epoch, features: { own_password_change: true } } });
      const lookup = route => { cookieSent ||= !!route.request().headers().cookie; return route.fulfill({ json: fixture.result, headers: { "Set-Cookie": "jobsim_password_fixture=late-cleared; Path=/; SameSite=Strict" } }); };
      await page.route("**/api/bootstrap", bootstrap); await page.route("**/api/auth/password-change-results", lookup);
      try {
        await page.evaluate(async id => window.cookiePasswordFixture.actions().checkOriginal(id, window.cookiePasswordFixture.proof), fixture.row.requestId);
        return !cookieSent && (await context.cookies(fixture.origin)).find(row => row.name === "jobsim_password_fixture")?.value === "successor" && await page.evaluate(family => window.passwordTest.auth.authSnapshot().auth.familyId === family, fixture.next);
      } finally { await page.unroute("**/api/bootstrap", bootstrap); await page.unroute("**/api/auth/password-change-results", lookup); }
    });
    await test("actual form clears proof fields immediately and requires explicit risk consent", async () => {
      await page.evaluate(() => { const f = window.passwordTest.create(); f.administrator = true; window.passwordFixture = f; f.render(); });
      await page.getByLabel("Current password", { exact: true }).fill("Original-password-123"); await page.getByLabel("New password", { exact: true }).fill("Replacement-password-456"); await page.getByLabel("Repeat new password", { exact: true }).fill("Replacement-password-456"); await page.getByLabel("Administrator authenticator code", { exact: true }).fill("123456");
      await page.locator("#password-fixture input[type=checkbox]").first().check();
      if (await page.getByLabel("Current password", { exact: true }).getAttribute("type") !== "text") return false;
      await page.locator("#password-fixture form button[value=create]").click();
      return page.evaluate(() => document.querySelectorAll("#password-fixture input[type=password]").length === 3 && [...document.querySelectorAll("#password-fixture input[type=password]")].every(input => !input.value) && !window.passwordFixture.log.some(r => r.route.endsWith("/password-change")));
    });
    await test("actual submit includes administrator OTP only on fresh authorization and exposes a truthful changed result", async () => {
      for (const [label, value] of [["Current password", "Original-password-123"], ["New password", "Replacement-password-456"], ["Repeat new password", "Replacement-password-456"], ["Administrator authenticator code", "123456"]]) await page.getByLabel(label, { exact: true }).fill(value);
      await page.locator("#password-fixture input[name=consent]").check(); await page.locator("#password-fixture form button[value=create]").click(); await page.getByText("Password changed; sign in again", { exact: true }).waitFor();
      return page.evaluate(() => window.passwordFixture.log.find(r => r.route.endsWith("/password-change")).body.otp === "123456" && window.passwordFixture.log.filter(r => r.route.endsWith("-results")).every(r => !("otp" in r.body)));
    });
    await test("ten languages and three themes keep the full form and original empty/reload state within mobile width", async () => {
      await page.setViewportSize({ width: 390, height: 844 });
      for (const locale of await page.evaluate(() => Object.keys(window.passwordTest.labels.passwordChangeLabels))) for (const theme of ["light", "dark", "system"]) {
        await page.evaluate(({ locale, theme }) => { const f = window.passwordTest.create(); f.locale = locale; window.passwordFixture = f; document.documentElement.lang = locale; document.documentElement.dataset.theme = theme === "system" ? "light" : theme; f.render(); }, { locale, theme });
        await page.waitForTimeout(20);
        if (!await page.evaluate(locale => { const labels = window.passwordTest.labels.passwordChangeLabels; return document.documentElement.scrollWidth <= innerWidth + 1 && Object.keys(labels[locale]).length === Object.keys(labels.en).length && Object.values(labels[locale]).every(value => typeof value === "string" && value.trim()); }, locale)) return false;
      }
      for (const [locale, theme] of [["zh-CN", "light"], ["en", "dark"], ["bo", "system"]]) {
        await page.evaluate(({ locale, theme }) => { const f = window.passwordFixture; f.locale = locale; document.documentElement.lang = locale; document.documentElement.dataset.theme = theme === "system" ? "light" : theme; f.render(); }, { locale, theme });
        await page.waitForTimeout(40); await page.screenshot({ path: path.join(out, "Password-" + locale + "-" + theme + ".png"), fullPage: true });
      }
      return true;
    });
    assert(JSON.stringify(hashes()) === JSON.stringify(before), "source changed during run");
    for (const file of sources) { const dest = path.join(out, "输入源码", file); fs.mkdirSync(path.dirname(dest), { recursive: true }); fs.copyFileSync(path.join(root, file), dest); }
    fs.writeFileSync(path.join(out, "结果.json"), JSON.stringify({ status: "passed", startedAt, finishedAt: new Date().toISOString(), checks, sourceSHA256: before, sourceUnchanged: true, scope: "Actual Chrome/React/IndexedDB/Web Locks; explicit response/password fixtures. Network API requests are aborted or fulfilled by explicit isolated browser routes. No live backend, CA, account or OS credential access.", apiAttempts }, null, 2));
    console.log(JSON.stringify({ status: "passed", groups: checks.length, report: out }));
  } catch (error) { fs.writeFileSync(path.join(out, "结果.json"), JSON.stringify({ status: "failed", startedAt, checks, sourceSHA256: before, error: String(error.stack || error) }, null, 2)); throw error; }
  finally { if (browser) await browser.close(); await vite.close(); }
})().catch(error => { console.error(error); process.exitCode = 1; });
