// Created by Jobs on 2026年11月16日，星期一.
// Runs only against an explicitly selected managed realm and a frozen Web build.
const fs = require("node:fs"), path = require("node:path"), https = require("node:https"), crypto = require("node:crypto"), assert = require("node:assert/strict");
const { chromium } = require(process.env.JOBSIM_PLAYWRIGHT_MODULE || "playwright");
if (!process.env.JOBSIM_SELF_TEST_ENV) throw Error("Select the managed restored realm explicitly");
const info = JSON.parse(fs.readFileSync(process.env.JOBSIM_SELF_TEST_ENV));
assert(info.restoredDraftRealm && info.allowOwnedRestartForTests && info.webDir);
assert.equal(info.serverURL, "https://127.0.0.1:54443");
assert.equal(info.binarySHA256, "dea37a66395b0b2c503ba934bafef327d3d87b407c73a10595e770f46948a2cd");
const root = path.resolve(__dirname, "../../../.."), stamp = Date.now() + "-" + process.pid;
const out = path.join(root, "项目相关报告/测试/前端/Web/20261116-S2本人状态", "HTTPS-" + stamp);
fs.mkdirSync(out, { recursive: true });
assert(Number(fs.statfsSync(out).bavail) * Number(fs.statfsSync(out).bsize) >= 4 * 1024 ** 3, "4GiB operator disk gate");
const fixtureFile = path.join(info.runDir, "Secrets", "web-self-state-" + stamp + ".json");
const agent = new https.Agent({ ca: fs.readFileSync(info.caPath) });
const cert = new crypto.X509Certificate(fs.readFileSync(info.tlsCert));
const pin = crypto.createHash("sha256").update(cert.publicKey.export({ type: "spki", format: "der" })).digest("base64");
const hash = value => crypto.createHash("sha256").update(value).digest("hex");
const candidateJS = /assets\/(index-[^" ]+\.js)/.exec(fs.readFileSync(path.join(info.webDir, "index.html"), "utf8"))?.[1];
assert(candidateJS, "Frozen candidate JS required");
const candidateSHA = hash(fs.readFileSync(path.join(info.webDir, "assets", candidateJS)));
const sources = fs.readdirSync(path.join(root, "代码/前端/Web/Src")).filter(name => /\.(tsx?|css)$/.test(name)).map(name => "代码/前端/Web/Src/" + name);
sources.push(path.relative(root, __filename));
const hashes = () => Object.fromEntries(sources.map(file => [file, hash(fs.readFileSync(path.join(root, file)))]));
const sourceSHA = hashes(), checks = [], cleanup = [], network = [], pageErrors = [], controls = [], browserSessions = [];
const password = "JobsIM isolated Web " + crypto.randomBytes(12).toString("hex");
let browser, browserContext, page, secondContext, secondPage, group, a, b, failure, dropped = false, droppedBodySHA = "", droppedRequestId = "", matchingPuts = 0, originalDraftBody;
const saveFixture = () => fs.writeFileSync(fixtureFile, JSON.stringify({ password, controls, browserSessions, group, run: stamp }, null, 2), { mode: 0o600 });
class HTTPError extends Error { constructor(status, code) { super(status + "/" + code); this.status = status; this.code = code; } }
function request(route, method = "GET", body, token, headers = {}) {
  return new Promise((resolve, reject) => {
    const q = https.request(info.serverURL + "/api" + route, { method, agent, headers: { "Content-Type": "application/json", ...(token ? { Authorization: "Bearer " + token } : {}), ...headers } }, response => {
      const chunks = [];
      response.on("data", chunk => chunks.push(chunk));
      response.on("end", () => {
        try {
          const data = JSON.parse(Buffer.concat(chunks).toString() || "{}");
          network.push({ method, route: route.split("?")[0], status: response.statusCode, errorCode: data.error?.code });
          if (response.statusCode >= 400) reject(new HTTPError(response.statusCode, data.error?.code || "HTTP_ERROR"));
          else resolve(data);
        } catch (error) { reject(error); }
      });
    });
    q.setTimeout(15000, () => q.destroy(Error("bounded fixture HTTPS timeout")));
    q.on("error", reject); q.end(body === undefined ? undefined : JSON.stringify(body));
  });
}
async function until(action, reason, timeout = 30000) {
  const end = Date.now() + timeout;
  while (Date.now() < end) {
    const value = await action(); if (value) return value;
    await new Promise(resolve => setTimeout(resolve, 150));
  }
  throw Error(reason);
}
async function state(targetPage) {
  return targetPage.evaluate(() => new Promise((resolve, reject) => {
    const q = indexedDB.open("JobsIM", 1); q.onerror = () => reject(q.error);
    q.onsuccess = () => {
      const db = q.result, tx = db.transaction("spaces"), store = tx.objectStore("spaces"), keys = store.getAllKeys(), values = store.getAll();
      tx.oncomplete = () => { const rows = Object.fromEntries(keys.result.map((key, i) => [key, values.result[i]])); db.close(); resolve(rows); };
      tx.onerror = () => reject(tx.error);
    };
  }));
}
const syncPair = rows => Object.entries(rows).find(([key, value]) => key.startsWith("sync-state|") && value.protocolSchema === "2" && !value.restriction && value.cursor);
const selfPanel = targetPage => targetPage.getByRole("region", { name: "My synced state" });
async function openDetails(targetPage) {
  if (!await selfPanel(targetPage).isVisible().catch(() => false)) await targetPage.locator(".thread-header .header-actions").getByRole("button", { name: "Conversation details", exact: true }).click();
  await selfPanel(targetPage).getByRole("checkbox", { name: "Pin", exact: true }).waitFor();
}
async function waitPanelIdle(targetPage) {
  await until(async () => !await selfPanel(targetPage).getByRole("button", { name: "Reload", exact: true }).isDisabled(), "panel remained busy");
}
async function login(targetContext, name) {
  await targetContext.addInitScript(() => { localStorage.setItem("jobsim.locale", JSON.stringify("en")); });
  const p = await targetContext.newPage(); p.on("pageerror", e => pageErrors.push(e.message));
  if (!page) page = p; else secondPage = p;
  p.on("response", response => {
    if (/\/api\/auth\/(login|register|refresh)$/.test(new URL(response.url()).pathname)) void response.json().then(value => { network.push({ client: "browser", route: new URL(response.url()).pathname, method: response.request().method(), status: response.status(), errorCode: value.error?.code }); if (Object.hasOwn(value, "token") || Object.hasOwn(value, "refreshToken")) pageErrors.push("BFF_EXPOSED_TOKEN"); }).catch(() => {});
  });
  await p.goto(info.serverURL, { waitUntil: "domcontentloaded" });
  await p.getByRole("button", { name: "Sign in", exact: true }).first().click();
  await p.locator('.modal input[name="username"]').waitFor();
  await p.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
  await p.locator('.modal input[name="username"]').fill(name);
  await p.locator('.modal input[name="password"]').fill(password);
  assert.equal(await p.locator('.modal input[name="username"]').inputValue(), name, "login username changed during modal focus");
  assert.equal(await p.locator('.modal input[name="password"]').inputValue(), password, "login password field was not filled");
  const signInResponse = p.waitForResponse(response => new URL(response.url()).pathname === "/api/auth/login" && response.request().method() === "POST");
  await p.locator(".modal form").getByRole("button", { name: "Sign in", exact: true }).click();
  const loginResponse = await signInResponse, issued = await loginResponse.json();
  assert.equal(loginResponse.status(), 200, "browser login " + (issued.error?.code || "HTTP_ERROR"));
  assert(issued.familyId && issued.deviceId && !Object.hasOwn(issued, "token") && !Object.hasOwn(issued, "refreshToken"));
  p.jobsimPublicSession = { familyId: issued.familyId, deviceId: issued.deviceId, username: name };
  browserSessions.push(p.jobsimPublicSession); saveFixture();
  await until(async () => !!syncPair(await state(p)), "S2 durable baseline not ready", 45000);
  const metadata = await p.evaluate(() => JSON.parse(sessionStorage.getItem("jobsim.auth.context") || localStorage.getItem("jobsim.auth.active") || "null"));
  assert(metadata?.familyId && metadata.deviceId);
  assert.equal(metadata.familyId, issued.familyId); assert.equal(metadata.deviceId, issued.deviceId);
  const context = await p.evaluate(async family => { const r = await fetch("/api/auth/context", { credentials: "include", headers: { "X-JobsIM-Client": "web", "X-JobsIM-Auth-Family": family } }); if (!r.ok) throw Error("public context unavailable"); return r.json(); }, metadata.familyId);
  assert(!Object.hasOwn(context, "token") && !Object.hasOwn(context, "refreshToken"));
  await p.getByRole("combobox", { name: "All conversations", exact: true }).selectOption("allChats");
  await p.locator(".conversation-row").filter({ hasText: group.title }).click();
  await p.locator(".composer textarea").waitFor();
  return p;
}
async function check(name, action) { await action(); checks.push({ name, status: "passed" }); console.log("PASS " + name); }

(async () => {
  const startedAt = new Date().toISOString();
  try {
    const boot = await request("/bootstrap");
    assert.equal(boot.deploymentId, info.deploymentId); assert.equal(boot.restoreEpoch, info.restoreEpoch);
    assert(boot.features.sync_self_state && boot.features.cloud_drafts);
    for (const suffix of ["a", "b"]) {
      const actor = await request("/auth/register", "POST", { username: "wss_" + Date.now().toString(36) + "_" + suffix, password, requestId: crypto.randomUUID(), deviceName: "Web S2 isolated control " + suffix });
      controls.push(actor); saveFixture();
    }
    [a, b] = controls;
    const invitation = await request("/contacts/requests", "POST", { userId: b.user.id, requestId: crypto.randomUUID() }, a.token);
    await request("/contacts/requests/" + invitation.id + "/accept", "POST", {}, b.token);
    group = await request("/conversations", "POST", { kind: "group", title: "Web S2 " + stamp, memberIds: [b.user.id], requestId: crypto.randomUUID() }, a.token); saveFixture();
    const peerView = await request("/conversations/" + group.id, "GET", undefined, b.token), fixtures = [];
    for (let index = 0; index < 40; index++) fixtures.push(await request("/conversations/" + group.id + "/messages", "POST", { clientMsgId: crypto.randomUUID(), type: "text", text: "Visible observation fixture " + index + " · independent line\nSecond line " + index, restoreEpoch: boot.restoreEpoch, sendAdmission: peerView.sendAdmission }, b.token));
    const initialCloud = await request("/conversations/" + group.id + "/draft", "GET", undefined, a.token);
    await request("/conversations/" + group.id + "/draft", "PUT", { requestId: crypto.randomUUID(), revisionEpoch: boot.restoreEpoch, expectedVersion: initialCloud.version, draftAdmission: initialCloud.draftAdmission, ownedText: "REMOTE_CLOUD_" + stamp, reply: null }, a.token);
    browser = await chromium.launch({ headless: true, executablePath: process.env.BROWSER_EXECUTABLE || "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", args: ["--ignore-certificate-errors-spki-list=" + pin] });
    browserContext = await browser.newContext({ viewport: { width: 1440, height: 900 } });
    page = await login(browserContext, a.user.username);
    await check("frozen production BFF logs in and installs schema2 without exposing bearer credentials", async () => {
      assert(await page.locator('script[src*="' + candidateJS + '"]').count());
      const cookies = await browserContext.cookies(); assert(cookies.some(row => row.httpOnly && row.secure));
      assert(!pageErrors.includes("BFF_EXPOSED_TOKEN")); assert(syncPair(await state(page)));
    });
    let firstStore;
    await check("actual visible observations retain the unseen prefix hole", async () => {
      await page.locator(".thread-body").evaluate(box => { box.scrollTop = box.scrollHeight; box.dispatchEvent(new Event("scroll", { bubbles: true })); });
      await page.bringToFront();
      const read = await until(async () => { const r = await request("/conversations/" + group.id + "/read", "GET", undefined, a.token); return BigInt(r.unreadCount) > 0n && BigInt(r.unreadCount) < 40n ? r : false; }, "visible message observations did not reach source");
      assert.equal(read.readPoint.sequence, "0");
      const [key, value] = syncPair(await state(page)); firstStore = { key, physical: value.storeGeneration };
    });
    await check("a full-screen modal never observes a newly arrived background message", async () => {
      await page.locator(".thread-header .header-actions").getByRole("button", { name: "Search messages or people", exact: true }).click();
      await page.locator(".modal").waitFor(); await page.waitForTimeout(500);
      const before = await request("/conversations/" + group.id + "/read", "GET", undefined, a.token);
      const incoming = await request("/conversations/" + group.id + "/messages", "POST", { clientMsgId: crypto.randomUUID(), type: "text", text: "OCCLUDED_NEW_MESSAGE_" + stamp, restoreEpoch: boot.restoreEpoch, sendAdmission: peerView.sendAdmission }, b.token);
      await until(async () => Object.entries(await state(page)).some(([key, value]) => !key.includes("|") ? false : value?.messages?.[group.id]?.some(row => row.id === incoming.id)), "new background message was not hydrated");
      const blocked = await request("/conversations/" + group.id + "/read", "GET", undefined, a.token);
      assert.equal(BigInt(blocked.unreadCount), BigInt(before.unreadCount) + 1n);
      await page.locator(".modal").getByRole("button", { name: "Close", exact: true }).click();
      await until(async () => BigInt((await request("/conversations/" + group.id + "/read", "GET", undefined, a.token)).unreadCount) < BigInt(blocked.unreadCount), "visible message was not observed after modal closed");
    });
    await openDetails(page);
    await check("view controls preserve manual unread independently and archived hidden chats remain recoverable", async () => {
      for (const name of ["Mark unread", "Archived", "Hidden", "Pin"]) {
        await waitPanelIdle(page); await selfPanel(page).getByRole("checkbox", { name, exact: true }).click();
        const field = { "Mark unread": "manualUnread", Archived: "archived", Hidden: "hidden", Pin: "pinnedRank" }[name];
        await until(async () => { const view = await request("/conversations/" + group.id + "/view", "GET", undefined, a.token); return field === "pinnedRank" ? view[field] === 0 : view[field] === true; }, "view action not committed: " + name);
      }
      await page.getByRole("combobox", { name: "All conversations", exact: true }).selectOption("hidden");
      assert(await page.locator(".conversation-row").filter({ hasText: group.title }).count());
      await page.getByRole("combobox", { name: "All conversations", exact: true }).selectOption("archived");
      assert(await page.locator(".conversation-row").filter({ hasText: group.title }).count());
      await page.getByRole("combobox", { name: "All conversations", exact: true }).selectOption("allChats");
      assert.equal((await request("/conversations/" + group.id + "/view", "GET", undefined, a.token)).manualUnread, true);
      await waitPanelIdle(page);
    });
    await check("cloud text is separate from the live local editor", async () => {
      await page.locator(".composer textarea").fill("LOCAL_EDIT_" + stamp);
      await selfPanel(page).getByText("REMOTE_CLOUD_" + stamp, { exact: true }).waitFor();
      assert.equal(await page.locator(".composer textarea").inputValue(), "LOCAL_EDIT_" + stamp);
    });
    await check("changed cloud version refreshes for review without publishing the old editor intent", async () => {
      const old = await request("/conversations/" + group.id + "/draft", "GET", undefined, a.token);
      const text = "REMOTE_CHANGED_" + stamp;
      await request("/conversations/" + group.id + "/draft", "PUT", { requestId: crypto.randomUUID(), revisionEpoch: boot.restoreEpoch, expectedVersion: old.version, draftAdmission: old.draftAdmission, ownedText: text, reply: null }, a.token);
      let puts = 0; const watch = req => { if (req.method() === "PUT" && new URL(req.url()).pathname === "/api/conversations/" + group.id + "/draft") puts++; }; page.on("request", watch);
      await waitPanelIdle(page); await selfPanel(page).getByRole("button", { name: "Save to cloud", exact: true }).click();
      await selfPanel(page).getByText(text, { exact: true }).waitFor(); await waitPanelIdle(page); page.off("request", watch);
      assert.equal(puts, 0); assert.equal((await request("/conversations/" + group.id + "/draft", "GET", undefined, a.token)).ownedText, text);
      assert.equal(await page.locator(".composer textarea").inputValue(), "LOCAL_EDIT_" + stamp);
    });
    const originalText = "LOST_RESPONSE_OWN_TEXT_" + stamp;
    await page.route(info.serverURL + "/api/conversations/" + group.id + "/draft", async route => {
      const req = route.request(), body = req.postDataJSON();
      if (req.method() !== "PUT" || body?.ownedText !== originalText) return route.continue();
      matchingPuts++;
      if (dropped) return route.continue();
      droppedBodySHA = hash(req.postDataBuffer()); droppedRequestId = body.requestId;
      const headers = await req.allHeaders();
      const committed = await new Promise((resolve, reject) => {
        const outgoing = https.request(req.url(), { method: req.method(), agent, headers }, response => {
          const bytes = []; response.on("data", byte => bytes.push(byte)); response.on("end", () => resolve({ status: response.statusCode, body: JSON.parse(Buffer.concat(bytes).toString()) }));
        });
        outgoing.setTimeout(15000, () => outgoing.destroy(Error("bounded strict response-drop proxy timeout")));
        outgoing.on("error", reject); outgoing.end(req.postDataBuffer());
      });
      assert.equal(committed.status, 200); assert.equal(committed.body.status, "applied"); dropped = true;
      return route.abort("failed");
    });
    await check("lost cloud save survives cold reload and resolves original readonly proof without a second PUT", async () => {
      await page.locator(".composer textarea").fill(originalText); await waitPanelIdle(page);
      await selfPanel(page).getByRole("button", { name: "Save to cloud", exact: true }).click();
      await until(() => dropped, "controlled response drop did not happen");
      await until(async () => Object.entries(await state(page)).some(([key, rows]) => key.startsWith("self-state-operations|") && rows.some(row => row.requestId === droppedRequestId && row.status === "pending" && hash(row.originalBody) === droppedBodySHA)), "original pending intent not durable");
      await page.reload({ waitUntil: "domcontentloaded" });
      await until(async () => !!syncPair(await state(page)), "cold S2 baseline not ready");
      await page.getByRole("combobox", { name: "All conversations", exact: true }).selectOption("allChats");
      await page.locator(".conversation-row").filter({ hasText: group.title }).click(); await openDetails(page);
      await selfPanel(page).getByRole("button", { name: /Check and retry original request/ }).click();
      await until(async () => Object.entries(await state(page)).some(([key, rows]) => key.startsWith("self-state-operations|") && rows.some(row => row.requestId === droppedRequestId && row.status === "applied" && hash(row.originalBody) === droppedBodySHA)), "original result was not confirmed");
      const rows = await state(page); const found = syncPair(rows); assert.equal(found[1].storeGeneration, firstStore.physical);
      originalDraftBody = Object.entries(rows).find(([key]) => key.startsWith("self-state-operations|"))[1].find(row => row.requestId === droppedRequestId).originalBody;
      assert.equal(matchingPuts, 1);
      assert.equal(await page.locator(".composer textarea").inputValue(), originalText);
    });
    await check("a second real browser sees cloud text while retaining its independent local input", async () => {
      secondContext = await browser.newContext({ viewport: { width: 1200, height: 900 } }); secondPage = await login(secondContext, a.user.username);
      await secondPage.locator(".composer textarea").fill("SECOND_LOCAL_" + stamp); await openDetails(secondPage);
      await selfPanel(secondPage).getByText(originalText, { exact: true }).waitFor();
      assert.equal(await secondPage.locator(".composer textarea").inputValue(), "SECOND_LOCAL_" + stamp);
      await selfPanel(secondPage).getByRole("button", { name: "Use this text", exact: true }).click();
      await until(async () => await secondPage.locator(".composer textarea").inputValue() === originalText, "explicit cloud adoption did not update local editor");
      assert(!(await state(secondPage))["outbox|" + syncPair(await state(secondPage))[0].slice("sync-state|".length)]?.length);
    });
    await check("clear cloud draft preserves local input and old proof reports current false", async () => {
      await waitPanelIdle(secondPage); await selfPanel(secondPage).getByRole("button", { name: "Clear cloud draft", exact: true }).click();
      await until(async () => (await request("/conversations/" + group.id + "/draft", "GET", undefined, a.token)).contentState === "empty", "cloud clear not committed");
      assert.equal(await secondPage.locator(".composer textarea").inputValue(), originalText);
      const proof = await request("/conversations/" + group.id + "/draft-results", "POST", { method: "PUT", body: JSON.parse(originalDraftBody) }, a.token);
      assert.equal(proof.current, false); assert.equal(proof.status, "applied");
    });
    await check("adoption clears old edit mode and mentions before a normal new message send", async () => {
      await page.bringToFront();
      const own = await request("/conversations/" + group.id + "/messages", "POST", { clientMsgId: crypto.randomUUID(), type: "text", text: "ORIGINAL_EDIT_TARGET_" + stamp, restoreEpoch: boot.restoreEpoch, sendAdmission: group.sendAdmission }, a.token);
      await page.locator("#message-" + own.id).waitFor(); await page.locator("#message-" + own.id).hover();
      await page.locator("#message-" + own.id).getByRole("button", { name: "Manage", exact: true }).click();
      await page.locator("#message-" + own.id).getByRole("button", { name: "Edit", exact: true }).click();
      await page.locator(".composer-tools").getByRole("button", { name: "Mention members", exact: true }).click();
      await page.locator(".modal").getByRole("checkbox", { name: "Mention everyone", exact: true }).check();
      await page.locator(".modal").getByRole("button", { name: "Close", exact: true }).click();
      const old = await request("/conversations/" + group.id + "/draft", "GET", undefined, a.token), text = "ADOPT_NORMAL_NEW_MESSAGE_" + stamp;
      await request("/conversations/" + group.id + "/draft", "PUT", { requestId: crypto.randomUUID(), revisionEpoch: boot.restoreEpoch, expectedVersion: old.version, draftAdmission: old.draftAdmission, ownedText: text, reply: null }, a.token);
      await openDetails(page); await waitPanelIdle(page); await selfPanel(page).getByRole("button", { name: "Reload", exact: true }).click();
      await selfPanel(page).getByText(text, { exact: true }).waitFor(); await selfPanel(page).getByRole("button", { name: "Use this text", exact: true }).click();
      await until(async () => await page.locator(".composer textarea").inputValue() === text, "adoption did not replace local text");
      assert.equal(await page.locator(".composer-reply").count(), 0);
      const sentResponse = page.waitForResponse(response => new URL(response.url()).pathname === "/api/conversations/" + group.id + "/messages" && response.request().method() === "POST");
      await page.locator(".composer").getByRole("button", { name: "Send", exact: true }).click();
      const sent = await (await sentResponse).json(); assert.equal(sent.text, text); assert.notEqual(sent.id, own.id); assert(!sent.mentionAll && !sent.mentionedUserIds?.length && !sent.replyToId);
      assert.equal((await request("/messages/" + own.id, "GET", undefined, a.token)).text, own.text);
    });
    await check("synced font is applied to the actual production UI", async () => {
      const pref = await request("/me/preferences", "GET", undefined, a.token);
      await request("/me/preferences", "PATCH", { requestId: crypto.randomUUID(), revisionEpoch: boot.restoreEpoch, expectedVersion: pref.version, fontPreset: "large" }, a.token);
      await until(async () => await secondPage.evaluate(() => document.documentElement.dataset.font) === "large", "server font not applied");
      await secondPage.screenshot({ path: path.join(out, "生产界面-大字号.png"), fullPage: true });
    });
    assert.deepEqual(hashes(), sourceSHA); assert.equal(hash(fs.readFileSync(path.join(info.webDir, "assets", candidateJS))), candidateSHA);
  } catch (error) {
    failure = { message: String(error.message), stack: String(error.stack) };
    const failedPage = secondPage ?? page;
    if (failedPage && !failedPage.isClosed()) await failedPage.screenshot({ path: path.join(out, "当场失败.png"), fullPage: true }).catch(() => {});
  } finally {
    // Browser logout must close its original device while control sessions can verify it.
    for (const [p, index] of [[secondPage, 1], [page, 0]]) {
      if (!p || p.isClosed()) continue;
      const session = p.jobsimPublicSession ?? browserSessions[index];
      if (!session) { cleanup.push({ kind: "browser", noSuccessfulFamilyObserved: true }); continue; }
      try {
        const result = await p.evaluate(async family => {
          const challenge = await fetch("/api/auth/csrf", { credentials: "include", headers: { "X-JobsIM-Client": "web", "X-JobsIM-Auth-Family": family } });
          if (!challenge.ok) return { ok: false, status: challenge.status };
          const { csrfToken } = await challenge.json();
          const response = await fetch("/api/auth/logout", { method: "POST", credentials: "include", headers: { "X-JobsIM-Client": "web", "X-JobsIM-Auth-Family": family, "X-CSRF-Token": csrfToken, "Content-Type": "application/json" }, body: "{}" });
          return { ok: response.ok, status: response.status };
        }, session?.familyId);
        const devices = await request("/auth/sessions", "GET", undefined, a.token);
        const absent = session && devices.items.every(row => typeof row.deviceId === "string") && !devices.items.some(row => row.deviceId === session.deviceId);
        cleanup.push({ kind: "browser", familyId: session?.familyId, logoutStatus: result.status, activeDeviceAbsent: !!absent });
        if (!result.ok || !absent) failure ||= { message: "browser original family closure not verified" };
      } catch (error) { cleanup.push({ kind: "browser", error: String(error.message) }); failure ||= { message: "browser cleanup failed" }; }
    }
    for (const actor of controls) {
      try {
        await request("/auth/logout", "POST", {}, actor.token); let rejected = false;
        try { await request("/me", "GET", undefined, actor.token); } catch (error) { rejected = error.status === 401; }
        cleanup.push({ kind: "control", familyId: actor.familyId, logoutStatus: 200, originalAT401: rejected });
        if (!rejected) failure ||= { message: "control original AT was not rejected" };
      } catch (error) { cleanup.push({ kind: "control", error: String(error.message) }); failure ||= { message: "control cleanup failed" }; }
    }
    await secondContext?.close().catch(() => {}); await browserContext?.close().catch(() => {}); await browser?.close().catch(() => {}); agent.destroy();
    fs.writeFileSync(path.join(out, "结果.json"), JSON.stringify({ startedAt, finishedAt: new Date().toISOString(), status: failure ? "failed" : "passed", candidateJS, candidateSHA256: candidateSHA, deploymentId: info.deploymentId, restoreEpoch: info.restoreEpoch, backendSHA256: info.binarySHA256, strictNodeCA: true, browserTrust: "Only the explicitly selected local certificate SPKI; no system trust or global TLS bypass", checks, controlledFault: { dropped, originalBodySHA256: droppedBodySHA, matchingPuts }, cleanup, network, pageErrors, sourceSHA256: sourceSHA, sourceUnchanged: JSON.stringify(sourceSHA) === JSON.stringify(hashes()), failure }, null, 2));
    if (failure) throw Error(failure.message);
  }
})().catch(error => { console.error(error.message); process.exitCode = 1; });
