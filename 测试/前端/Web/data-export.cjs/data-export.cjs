// Created by Jobs on 2026年11月16日，星期一.
const fs = require("node:fs"), path = require("node:path"), os = require("node:os"), crypto = require("node:crypto"), { pathToFileURL } = require("node:url");
const { chromium } = require(process.env.JOBSIM_PLAYWRIGHT_MODULE || "playwright");
const root = path.resolve(__dirname, "../../../.."), web = path.join(root, "代码/前端/Web");
const sources = ["App.tsx", "api.ts", "DataExportActions.ts", "DataExportArchive.ts", "DataExportLabels.ts", "DataExportPanel.tsx", "DataExportProtocol.ts", "DataExportStore.ts", "styles.css"].map(name => "代码/前端/Web/Src/" + name).concat("测试/前端/Web/data-export.cjs/data-export.cjs");
const hash = () => Object.fromEntries(sources.map(file => [file, crypto.createHash("sha256").update(fs.readFileSync(path.join(root, file))).digest("hex")]));
const out = process.env.JOBSIM_EXPORT_REPORT || path.join(root, "项目相关报告/测试/前端/Web/正式本人导出", Date.now() + "-" + process.pid);
const startedAt = new Date().toISOString(), before = hash(), checks = [];
const assert = (value, reason) => { if (!value) throw Error(reason); };
const space = fs.statfsSync(root); if (Number(space.bavail) * Number(space.bsize) < 4 * 1024 ** 3) throw Error("At least 4GiB free disk is required before browser/Vite startup");
if (fs.existsSync(out)) throw Error("Choose a new report directory");
fs.mkdirSync(out, { recursive: true });
(async () => {
  const work = fs.mkdtempSync(path.join(process.env.JOBSIM_EXPORT_WORK || os.tmpdir(), "jobsim-data-export-"));
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
      const [actions, protocol, store, archive, labels, api, auth, database, panel, i18n] = await Promise.all([
        import("/Src/DataExportActions.ts"), import("/Src/DataExportProtocol.ts"), import("/Src/DataExportStore.ts"), import("/Src/DataExportArchive.ts"), import("/Src/DataExportLabels.ts"), import("/Src/api.ts"), import("/Src/AuthSession.ts"), import("/Src/StorageDatabase.ts"), import("/Src/DataExportPanel.tsx"), import("/Src/i18n.ts"),
      ]);
      const resources = performance.getEntriesByType("resource").map(row => row.name), reactURL = resources.find(url => /\/react\.js\?/.test(url)), domURL = resources.find(url => /\/react-dom_client\.js\?/.test(url));
      if (!reactURL || !domURL) throw Error("Real React modules absent");
      const R = await import(reactURL), D = await import(domURL), React = R.default ?? R, dom = D.default ?? D, db = await database.storageDatabase;
      const read = key => new Promise((resolve, reject) => { const tx = db.transaction("spaces"), q = tx.objectStore("spaces").get(key); tx.oncomplete = () => resolve(q.result); tx.onerror = () => reject(tx.error); });
      const write = rows => new Promise((resolve, reject) => { const tx = db.transaction("spaces", "readwrite", { durability: "strict" }); for (const [key, value] of rows) tx.objectStore("spaces").put(value, key); tx.oncomplete = resolve; tx.onerror = () => reject(tx.error); });
      const container = document.createElement("div"); container.id = "export-fixture"; container.className = "settings-content"; container.style.cssText = "max-width:720px;margin:auto;background:var(--surface);color:var(--text)";
      document.getElementById("root").style.display = "none"; document.body.append(container);
      const domRoot = dom.createRoot(container), downloads = [];
      HTMLAnchorElement.prototype.click = function () { downloads.push({ name: this.download, href: this.href }); };
      const create = async () => {
        const f = { current: true, enabled: true, log: [], facts: {}, mode: "ok", locale: "en", connected: 0, reloaded: 0 };
        f.identity = { scopeId: crypto.randomUUID(), userId: crypto.randomUUID(), deviceId: crypto.randomUUID(), epoch: "3" };
        f.namespace = [location.origin, f.identity.scopeId, f.identity.userId, "personal", "3", f.identity.deviceId].join("|");
        await write([["auth-active|" + f.namespace.split("|").slice(0, 4).join("|"), f.namespace]]);
        f.view = id => ({ job: { id, artifactId: crypto.randomUUID(), restoreEpoch: "3", buildGeneration: "9007199254740993", state: "ready", expiresAt: new Date(Date.now() + 3600000).toISOString(), cleanupState: "published" }, format: "zip", scope: "personal_current", coverage: ["forward_placeholders_only"], exclusions: ["access_and_refresh_tokens"] });
        f.request = async (path, init = {}) => {
          const method = init.method || "GET", body = init.body && JSON.parse(init.body);
          f.log.push({ path, method, raw: init.body });
          if (path === "/bootstrap") return { deploymentId: f.identity.scopeId, restoreEpoch: "3", features: { chat_export: f.enabled } };
          if (path === "/me/data-export-results") {
            const found = f.facts[body.requestId];
            if (!found) throw new api.ApiError(404, "EXPORT_OPERATION_NOT_FOUND", "injected original absent");
            return structuredClone(found);
          }
          if (path === "/me/data-exports") {
            const originals = await read("data-exports|" + f.namespace);
            if (!originals?.some(row => row.originalBody === init.body && row.bodySHA256)) throw Error("WRITE_BEFORE_DURABLE_ORIGINAL");
            if (f.mode === "unreached") throw new api.ApiError(503, "INJECTED_UNKNOWN", "injected before receipt");
            const view = f.view(crypto.randomUUID()); f.facts[body.requestId] = view;
            if (f.mode === "lost") throw new api.ApiError(503, "INJECTED_UNKNOWN", "injected after commit");
            if (f.mode === "held") await new Promise(resolve => { f.release = resolve; });
            return structuredClone(view);
          }
          const match = path.match(/^\/me\/data-exports\/([a-f0-9-]+)(\/(?:cancel|download-ticket))?$/);
          if (!match) throw Error("Unexpected route");
          const found = Object.values(f.facts).find(row => row.job.id === match[1]);
          if (!found || f.statusFails) throw new api.ApiError(503, "INJECTED_UNKNOWN", "injected current unknown");
          if (match[2] === "/cancel") {
            const originals = await read("data-exports|" + f.namespace);
            if (!originals.some(row => row.cancelBody === init.body && row.cancelSHA256)) throw Error("CANCEL_BEFORE_DURABLE_ORIGINAL");
            if (f.cancelUnknown) throw new api.ApiError(503, "INJECTED_UNKNOWN", "injected cancellation unknown");
            found.job.state = "cancelled"; found.job.cleanupState = "cleared";
          }
          if (match[2] === "/download-ticket") return { downloadToken: "RAM_ONLY_DOWNLOAD_TICKET_123456", expiresAt: new Date(Date.now() + 60000).toISOString(), format: "zip" };
          return structuredClone(found);
        };
        f.context = { namespace: f.namespace, identity: f.identity, isCurrent: () => f.current, request: f.request, file: async (path, init) => {
          f.fileHeaders = new Headers(init.headers); f.filePath = path;
          if (f.holdFile) await new Promise(resolve => { f.releaseFile = resolve; });
          return { blob: new Blob(["injected already-validated apiFile result"], { type: "application/zip" }), fileName: "jobsim-personal-data.zip" };
        } };
        f.actions = new actions.DataExportActions(f.context);
        f.render = () => domRoot.render(React.createElement(panel.default, { key: f.namespace, context: f.context, enabled: f.enabled, locale: f.locale, t: key => i18n.translate(f.locale, key), onConnect: () => f.connected++, onReload: () => f.reloaded++ }));
        return f;
      };
      window.exportTest = { actions, protocol, store, archive, labels: labels.dataExportLabels, api, auth, db, read, write, create, domRoot, React, panel, i18n, downloads };
    });
    const test = async (name, fn) => { assert(await page.evaluate(fn), name); checks.push(name); };
    await test("filename and complete ZIP validation reject traversal, controls, duplicates, truncated streams and trailing bytes", async () => {
      const t = window.exportTest, bytes = Uint8Array.from(atob("UEsDBBQAAAAAAGmZcF1Dv6ajAgAAAAIAAAANAAAAbWV0YWRhdGEuanNvbnt9UEsBAhQDFAAAAAAAaZlwXUO/pqMCAAAAAgAAAA0AAAAAAAAAAAAAAIABAAAAAG1ldGFkYXRhLmpzb25QSwUGAAAAAAEAAQA7AAAALQAAAAAA"), c => c.charCodeAt(0));
      t.zip = bytes;
      if (t.archive.exportFileName("attachment; filename=fallback.zip; filename*=UTF-8''my%20data.zip") !== "my data.zip") return false;
      for (const header of [null, "inline; filename=data.zip", 'attachment; filename="../data.zip"', 'attachment; filename="bad\\\\data.zip"', "attachment; filename=one.zip; filename=two.zip", "attachment; filename*=UTF-8''%00bad.zip", "attachment; filename*=UTF-8''%C2%85bad.zip", "attachment; filename*=UTF-8''%E2%80%AEbad.zip", "attachment; filename=not-a-zip.txt"]) {
        try { t.archive.exportFileName(header); return false; } catch { /* invalid server filename */ }
      }
      await t.archive.validateExportZIP(new Blob([bytes]));
      for (const blob of [new Blob([bytes.slice(0, -7)]), new Blob([bytes, "tail"]), new Blob(["not zip"])]) {
        try { await t.archive.validateExportZIP(blob); return false; } catch { /* incomplete container */ }
      }
      const oversizedDirectory = bytes.slice(); new DataView(oversizedDirectory.buffer).setUint32(bytes.length - 22 + 12, t.archive.exportDirectoryLimit + 1, true);
      try { await t.archive.validateExportZIP(new Blob([oversizedDirectory])); return false; } catch (error) { if (error.code !== "DATA_EXPORT_FILE_LIMIT") return false; }
      return true;
    });
    await test("actual apiFile uses cookie/family/header authentication, validates ZIP and rejects MIME/size/truncation before handoff", async () => {
      const t = window.exportTest, originalFetch = window.fetch, family = crypto.randomUUID(), scope = crypto.randomUUID();
      const auth = { familyId: family, refreshGeneration: "1", restoreEpoch: "3", sourceContextId: "personal", deploymentId: scope, userId: crypto.randomUUID(), username: "export_injected", deviceId: crypto.randomUUID(), accessExpiresAt: new Date(Date.now() + 600000).toISOString(), idleExpiresAt: new Date(Date.now() + 86400000).toISOString(), absoluteExpiresAt: new Date(Date.now() + 86400000).toISOString() };
      t.auth.configureAuthProtocol(true, scope); t.auth.rememberPublicAuth(auth);
      let mode = "ok", cancelled = false, requested = 0;
      window.fetch = async (url, init) => {
        if (url !== "/api/me/data-exports/" + family + "/download") return originalFetch(url, init);
        requested++;
        if (init.credentials !== "include" || init.headers.get("X-JobsIM-Auth-Family") !== family || init.headers.get("X-JobsIM-Download-Token") !== "RAM_TICKET_ONLY") throw Error("BFF_HEADER_MISSING");
        const headers = { "Content-Type": mode === "mime" ? "text/html" : "application/zip", "Content-Disposition": "attachment; filename=jobsim-personal-data.zip" };
        if (mode === "limit") headers["Content-Length"] = String(t.archive.exportZIPLimit + 1);
        if (mode === "declared") headers["Content-Length"] = String(t.zip.length + 1);
        const bytes = mode === "truncated" ? t.zip.slice(0, -3) : t.zip;
        return new Response(new ReadableStream({ start(c) { c.enqueue(bytes); if (mode === "mime" || mode === "limit") return; c.close(); }, cancel() { cancelled = true; } }), { headers });
      };
      try {
        const options = { headers: { "X-JobsIM-Download-Token": "RAM_TICKET_ONLY" } }, path = "/me/data-exports/" + family + "/download";
        const file = await t.api.apiFile(path, options); if (file.fileName !== "jobsim-personal-data.zip" || file.blob.size !== t.zip.length) return false;
        for (mode of ["mime", "limit", "truncated", "declared"]) { cancelled = false; try { await t.api.apiFile(path, options); return false; } catch { if (["mime", "limit"].includes(mode) && !cancelled) return false; } }
        return requested === 5;
      } finally { window.fetch = originalFetch; t.auth.clearPublicAuth(); t.auth.configureAuthProtocol(false, ""); }
    });
    await test("lost successful create is reconciled by original-body result with no second creating POST", async () => {
      const t = window.exportTest, f = await t.create(); f.mode = "lost";
      try { await f.actions.create(); return false; } catch { /* deliberately lost response */ }
      const original = (await f.actions.rows())[0], value = await f.actions.check(original.requestId);
      return !!value && f.log.filter(row => row.path === "/me/data-exports").length === 1 && (await f.actions.rows())[0].originalBody === original.originalBody;
    });
    await test("unreached original stays read-only on check and only explicit retry reuses its exact ID/body", async () => {
      const t = window.exportTest, f = await t.create(); f.mode = "unreached";
      try { await f.actions.create(); } catch { /* no server fact */ }
      const original = (await f.actions.rows())[0];
      if (await f.actions.check(original.requestId)) return false;
      if (f.log.filter(row => row.path === "/me/data-exports").length !== 1) return false;
      f.mode = "ok"; await f.actions.check(original.requestId, true);
      const posts = f.log.filter(row => row.path === "/me/data-exports");
      return posts.length === 2 && posts.every(row => row.raw === original.originalBody) && (await f.actions.rows()).length === 1;
    });
    await test("cancel original is durable and stable; checking an unknown cancellation never resends it", async () => {
      const t = window.exportTest, f = await t.create(); await f.actions.create(); const original = (await f.actions.rows())[0];
      f.cancelUnknown = true; try { await f.actions.cancel(original.requestId); } catch { /* unknown */ }
      const cancelled = (await f.actions.rows())[0]; await f.actions.status(original.requestId); await f.actions.check(original.requestId);
      if (f.log.filter(row => row.path.endsWith("/cancel")).length !== 1 || JSON.parse(cancelled.cancelBody).requestId === original.requestId) return false;
      f.cancelUnknown = false; await f.actions.cancel(original.requestId);
      return f.log.filter(row => row.path.endsWith("/cancel")).every(row => row.raw === cancelled.cancelBody);
    });
    await test("capability withdrawal, frozen namespace and corrupt body digest each block creating writes", async () => {
      const t = window.exportTest, f = await t.create(); f.enabled = false;
      try { await f.actions.create(); return false; } catch { /* denied */ }
      if (f.log.some(row => row.path === "/me/data-exports")) return false;
      f.enabled = true; const original = await t.store.addExportIntent(f.context);
      await t.write([["data-exports|" + f.namespace, [{ ...original, originalBody: original.originalBody + " " }]]]);
      try { await f.actions.check(original.requestId, true); return false; } catch { /* digest mismatch */ }
      await t.write([["data-exports|" + f.namespace, [original]], ["auth-frozen|" + f.namespace, true]]);
      try { await f.actions.check(original.requestId, true); return false; } catch { /* namespace retired */ }
      return !f.log.some(row => row.path === "/me/data-exports");
    });
    await test("capability withdrawal still allows original cancellation but blocks both new creation and downloads", async () => {
      const f = await window.exportTest.create(); await f.actions.create(); const id = (await f.actions.rows())[0].requestId;
      f.enabled = false;
      try { await f.actions.create(); return false; } catch { /* no new create */ }
      try { await f.actions.download(id); return false; } catch { /* no ticket */ }
      const result = await f.actions.cancel(id);
      return result.job.state === "cancelled" && f.log.filter(row => row.path === "/me/data-exports").length === 1 && !f.log.some(row => row.path.endsWith("/download-ticket"));
    });
    await test("late create from a retired family cannot bind a job or change the original payload", async () => {
      const t = window.exportTest, f = await t.create(); f.mode = "held";
      const pending = f.actions.create().then(() => false, () => true);
      while (!f.release) await new Promise(resolve => setTimeout(resolve, 5));
      f.current = false; f.release(); const rejected = await pending;
      const rows = await t.read("data-exports|" + f.namespace);
      return rejected && rows.length === 1 && !rows[0].jobId;
    });
    await test("Web Locks queued original retry is fenced before any write after context retirement", async () => {
      const t = window.exportTest, f = await t.create(); const original = await t.store.addExportIntent(f.context);
      let unlock; const lock = navigator.locks.request("jobsim-data-export|" + f.namespace, () => new Promise(resolve => { unlock = resolve; }));
      while (!unlock) await new Promise(resolve => setTimeout(resolve, 5));
      const pending = f.actions.check(original.requestId, true).then(() => false, () => true);
      f.current = false; unlock(); await lock;
      return await pending && !f.log.some(row => row.path === "/me/data-exports");
    });
    await test("strict metadata projection preserves decimal counters, drops unknown secrets and rejects wrong epoch/job", async () => {
      const t = window.exportTest, f = await t.create(), raw = f.view(crypto.randomUUID()); raw.secret = "NEVER_PERSIST"; raw.job.key = "NEVER_PERSIST";
      const projected = t.protocol.exportView(raw, f.identity);
      let rejected = 0;
      for (const bad of [{ ...raw, job: { ...raw.job, restoreEpoch: "2" } }, { ...raw, job: { ...raw.job, buildGeneration: 9007199254740993 } }]) try { t.protocol.exportView(bad, f.identity); } catch { rejected++; }
      try { t.protocol.exportView(raw, f.identity, crypto.randomUUID()); } catch { rejected++; }
      return projected.job.buildGeneration === "9007199254740993" && !JSON.stringify(projected).includes("NEVER_PERSIST") && rejected === 3;
    });
    await test("download ticket is RAM/header-only and never persisted in task originals", async () => {
      const t = window.exportTest, f = await t.create(); await f.actions.create(); const id = (await f.actions.rows())[0].requestId;
      await f.actions.download(id);
      return f.fileHeaders.get("X-JobsIM-Download-Token") === "RAM_ONLY_DOWNLOAD_TICKET_123456" && !f.filePath.includes("?") && !JSON.stringify(await t.read("data-exports|" + f.namespace)).match(/RAM_ONLY|key|token|NEVER_PERSIST/);
    });
    const mount = async () => {
      await page.evaluate(async () => { window.exportFixture = await window.exportTest.create(); window.exportFixture.render(); });
      await page.getByText("No exports yet", { exact: true }).waitFor();
    };
    await mount();
    await page.getByRole("button", { name: "Request export", exact: true }).click();
    await page.getByRole("button", { name: "Download ZIP", exact: true }).waitFor();
    await page.getByRole("button", { name: "Download ZIP", exact: true }).click();
    await page.getByText("The file was handed to your browser. Store it safely.", { exact: true }).waitFor();
    assert(await page.evaluate(() => window.exportTest.downloads.length === 1 && window.exportTest.downloads[0].name === "jobsim-personal-data.zip" && window.exportTest.downloads[0].href.startsWith("blob:")), "UI browser handoff failed");
    await page.evaluate(() => { window.exportFixture.context.file = async () => { throw new window.exportTest.archive.DataExportFileError("DATA_EXPORT_FILE_LIMIT"); }; });
    await page.getByRole("button", { name: "Download ZIP", exact: true }).click();
    await page.getByRole("alert").filter({ hasText: "This browser limits a download to 1GiB" }).waitFor();
    assert(await page.evaluate(() => window.exportTest.downloads.length === 1), "oversized file reached browser save sink");
    checks.push("browser download/ZIP-directory limits have a specific localized error and never hand oversized files to save");
    await page.getByRole("button", { name: "Cancel export", exact: true }).click();
    await page.getByRole("button", { name: "Keep export", exact: true }).click();
    assert(await page.evaluate(() => !window.exportFixture.log.some(row => row.path.endsWith("/cancel"))), "keep action cancelled export");
    await page.getByRole("button", { name: "Cancel export", exact: true }).click();
    await page.getByRole("button", { name: "Confirm cancellation", exact: true }).click();
    await page.getByText("Cancelled", { exact: true }).waitFor();
    assert(await page.getByRole("button", { name: "Download ZIP", exact: true }).count() === 0, "cancelled task still downloadable");
    checks.push("rendered task lifecycle creates, downloads via a short-lived Blob URL and requires explicit cancellation confirmation");
    await mount();
    await page.evaluate(() => { window.exportFixture.mode = "unreached"; });
    await page.getByRole("button", { name: "Request export", exact: true }).click();
    await page.getByRole("button", { name: "Retry original request", exact: true }).waitFor();
    await page.getByRole("button", { name: "Check original request", exact: true }).click();
    assert(await page.evaluate(() => window.exportFixture.log.filter(row => row.path === "/me/data-exports").length === 1), "UI check replayed POST");
    await page.evaluate(() => { window.exportFixture.mode = "ok"; });
    await page.getByRole("button", { name: "Retry original request", exact: true }).click();
    await page.getByRole("button", { name: "Download ZIP", exact: true }).waitFor();
    await page.evaluate(() => { window.exportFixture.statusFails = true; });
    await page.getByRole("button", { name: "Reload", exact: true }).first().click();
    await page.getByText("Needs checking", { exact: true }).waitFor();
    assert(await page.getByRole("button", { name: "Download ZIP", exact: true }).count() === 0, "unknown refresh left old download active");
    checks.push("rendered unknown task offers read-only check and explicit same-request retry; failed current GET clears its prior downloadable view");
    await mount();
    await page.getByRole("button", { name: "Request export", exact: true }).click();
    await page.getByRole("button", { name: "Download ZIP", exact: true }).waitFor();
    await page.evaluate(() => { window.exportFixture.holdFile = true; });
    const downloaded = await page.evaluate(() => window.exportTest.downloads.length);
    await page.getByRole("button", { name: "Download ZIP", exact: true }).click();
    await page.waitForFunction(() => typeof window.exportFixture.releaseFile === "function");
    await page.evaluate(async () => { const old = window.exportFixture; old.current = false; const next = await window.exportTest.create(); window.exportFixture = next; next.render(); old.releaseFile(); });
    await page.getByText("No exports yet", { exact: true }).waitFor();
    assert(await page.evaluate(n => window.exportTest.downloads.length === n, downloaded), "old family downloaded into new account");
    checks.push("unmounted old-family download completion cannot create a Blob URL or publish into the next account");
    await mount();
    await page.getByRole("button", { name: "Request export", exact: true }).click();
    await page.getByRole("button", { name: "Download ZIP", exact: true }).waitFor();
    await page.setViewportSize({ width: 390, height: 844 });
    for (const locale of await page.evaluate(() => Object.keys(window.exportTest.labels))) for (const theme of ["light", "dark", "system"]) {
      await page.evaluate(({ locale, theme }) => { const f = window.exportFixture; f.locale = locale; document.documentElement.lang = locale; document.documentElement.dataset.theme = theme === "system" ? "light" : theme; f.render(); }, { locale, theme });
      await page.waitForTimeout(20);
      assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), "mobile overflow " + locale + "/" + theme);
      assert(await page.evaluate(locale => { const a = window.exportTest.labels; return Object.keys(a[locale]).length === Object.keys(a.en).length && Object.values(a[locale]).every(value => typeof value === "string" && value.trim()); }, locale), "translation coverage " + locale);
    }
    for (const [locale, theme] of [["zh-CN", "light"], ["en", "dark"], ["bo", "system"]]) {
      await page.evaluate(({ locale, theme }) => { const f = window.exportFixture; f.locale = locale; document.documentElement.lang = locale; document.documentElement.dataset.theme = theme === "system" ? "light" : theme; f.render(); }, { locale, theme });
      await page.waitForTimeout(40); await page.screenshot({ path: path.join(out, "Export-" + locale + "-" + theme + ".png"), fullPage: true });
    }
    checks.push("ten complete languages inherit all three native Web theme settings without mobile horizontal overflow");
    await page.evaluate(() => { const f = window.exportFixture; f.locale = "en"; f.enabled = false; f.render(); });
    await page.getByText("New exports and downloads are unavailable. You can still check or cancel existing tasks.", { exact: true }).waitFor();
    assert(await page.getByRole("button", { name: "Request export", exact: true }).isDisabled() && await page.getByRole("button", { name: "Cancel export", exact: true }).isVisible() && await page.getByRole("button", { name: "Download ZIP", exact: true }).count() === 0, "capability withdrawal hid cleanup or enabled new work");
    checks.push("rendered capability withdrawal preserves existing original check/cancel controls while disabling create/download");
    await page.evaluate(() => { const f = window.exportFixture; window.exportTest.domRoot.render(window.exportTest.React.createElement(window.exportTest.panel.default, { key: "demo-empty", enabled: false, locale: "en", t: key => window.exportTest.i18n.translate("en", key), onConnect: () => f.connected++, onReload: () => f.reloaded++ })); });
    await page.getByText("Connect to use exports", { exact: true }).waitFor();
    assert(await page.getByRole("button", { name: "Request export", exact: true }).count() === 0 && await page.getByRole("button", { name: "Download ZIP", exact: true }).count() === 0, "unsupported/demo synthesized private export");
    checks.push("unsupported connection has a truthful empty/connect/reload state and no synthetic ZIP");
    assert(JSON.stringify(hash()) === JSON.stringify(before), "source changed during run");
    fs.writeFileSync(path.join(out, "结果.json"), JSON.stringify({ startedAt, finishedAt: new Date().toISOString(), status: "passed", checks, sourceSHA256: before, sourceUnchanged: true, apiAttempts, work, scope: "Actual Chrome/React/IndexedDB/Web Locks; injected personal-export request/file services and browser-save sink. No live server or OS filesystem persistence attestation." }, null, 2));
    process.stdout.write(JSON.stringify({ status: "passed", groups: checks.length, report: out }) + "\n");
  } catch (error) {
    fs.writeFileSync(path.join(out, "结果.json"), JSON.stringify({ startedAt, status: "failed", error: String(error.stack || error), checks, sourceSHA256: before }, null, 2));
    throw error;
  } finally { if (browser) await browser.close(); await vite.close(); }
})().catch(error => { process.stderr.write(String(error.stack || error) + "\n"); process.exitCode = 1; });
