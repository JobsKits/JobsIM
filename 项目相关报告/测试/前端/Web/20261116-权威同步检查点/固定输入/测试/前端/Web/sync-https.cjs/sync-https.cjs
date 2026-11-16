// Created by Jobs on 2026年11月16日，星期一.
// Creates only its own fixtures in an explicitly isolated synchronization realm.
const fs = require("node:fs"), path = require("node:path"), https = require("node:https");
const crypto = require("node:crypto"), assert = require("node:assert/strict");
const { chromium } = require(process.env.JOBSIM_PLAYWRIGHT_MODULE || "playwright");
if (!process.env.JOBSIM_SYNC_TEST_SEED) throw Error("Provide an explicit isolated synchronization seed");
const seed = JSON.parse(fs.readFileSync(process.env.JOBSIM_SYNC_TEST_SEED, "utf8"));
if (!seed.freshSyncRealm || !seed.persistentData) throw Error("An isolated persistent synchronization realm is required");
const server = seed.serverURL, agent = new https.Agent({ ca: fs.readFileSync(seed.caPath) });
const cert = new crypto.X509Certificate(fs.readFileSync(path.join(path.dirname(seed.caPath), "server.pem")));
const pin = crypto.createHash("sha256").update(cert.publicKey.export({ type: "spki", format: "der" })).digest("base64");
const out = process.env.JOBSIM_SYNC_REPORT || path.resolve(__dirname, "../../../../项目相关报告/测试/前端/Web/执行记录/sync-https-" + Date.now() + "-" + process.pid);
function native(route, method = "GET", body, token) {
  return new Promise((resolve, reject) => {
    const request = https.request(server + "/api" + route, { method, agent, headers: { "Content-Type": "application/json", ...(token ? { Authorization: "Bearer " + token } : {}) } }, response => {
      const chunks = [];
      response.on("data", chunk => chunks.push(chunk));
      response.on("end", () => {
        try {
          const value = JSON.parse(Buffer.concat(chunks).toString() || "{}");
          if (response.statusCode >= 400) reject(Error(response.statusCode + "/" + (value.error?.code || "HTTP_ERROR") + " " + route));
          else resolve(value);
        } catch { reject(Error("Invalid JSON " + route)); }
      });
    });
    request.setTimeout(30000, () => request.destroy(Error("Fixture request timed out")));
    request.on("error", reject);
    request.end(body === undefined ? undefined : JSON.stringify(body));
  });
}
const readStore = (page, keys) => page.evaluate(keys => new Promise((resolve, reject) => {
  const open = indexedDB.open("JobsIM", 1);
  open.onerror = () => reject(open.error);
  open.onsuccess = () => {
    const db = open.result, tx = db.transaction("spaces"), store = tx.objectStore("spaces");
    const requests = keys.map(key => store.get(key));
    tx.oncomplete = () => { resolve(requests.map(request => request.result)); db.close(); };
    tx.onerror = () => { reject(tx.error); db.close(); };
  };
}), keys);
async function until(run, explanation, timeout = 90000) {
  const end = Date.now() + timeout;
  while (Date.now() < end) {
    const result = await run();
    if (result) return result;
    await new Promise(resolve => setTimeout(resolve, 200));
  }
  throw Error(explanation);
}

(async () => {
  const boot = await native("/bootstrap");
  assert.equal(boot.features.authoritative_sync, true);
  assert.equal(boot.features.auth_refresh, true);
  const stamp = Date.now().toString(36), password = "JobsIM sync test " + crypto.randomBytes(12).toString("hex");
  const a = await native("/auth/register", "POST", { username: "web_sync_a_" + stamp, password, requestId: crypto.randomUUID(), deviceName: "Web sync fixture A" });
  const b = await native("/auth/register", "POST", { username: "web_sync_b_" + stamp, password, requestId: crypto.randomUUID(), deviceName: "Web sync fixture B" });
  const invitation = await native("/contacts/requests", "POST", { userId: b.user.id, requestId: crypto.randomUUID() }, a.token);
  await native("/contacts/requests/" + invitation.id + "/accept", "POST", {}, b.token);
  const title = "Sync HTTPS " + stamp;
  const group = await native("/conversations", "POST", { kind: "group", title, memberIds: [b.user.id], requestId: crypto.randomUUID() }, a.token);
  const fixtures = [];
  for (let start = 0; start < 96; start += 4) {
    const rows = await Promise.all(Array.from({ length: 4 }, (_, offset) => {
      const index = start + offset;
      return native("/conversations/" + group.id + "/messages", "POST", { clientMsgId: crypto.randomUUID(), type: "text", text: "Sync fixture " + stamp + " " + index + " <tag>&\u2028བོད་", restoreEpoch: boot.restoreEpoch, sendAdmission: group.sendAdmission }, a.token);
    }));
    fixtures.push(...rows);
  }
  const browser = await chromium.launch({ headless: true, executablePath: process.env.BROWSER_EXECUTABLE || "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", args: ["--ignore-certificate-errors-spki-list=" + pin] });
  const context = await browser.newContext({ viewport: { width: 1440, height: 960 } });
  const page = await context.newPage(), checks = [], errors = [], snapshotIDs = new Set();
  const pageReads = new Map();
  let blockSecondPage = true, blocked = 0, historyReads = 0, snapshots = 0;
  const unknownText = "Committed unknown UI send " + stamp;
  let unknownWrites = 0, unknownCommitted = false, unknownClientId = "", proxyFailure = "";
  page.on("pageerror", error => errors.push(error.message));
  context.on("request", request => {
    const url = new URL(request.url());
    if (request.method() === "GET" && /^\/api\/conversations\/[^/]+\/messages$/.test(url.pathname)) historyReads++;
    if (request.method() === "POST" && url.pathname === "/api/sync/snapshots") { snapshots++; snapshotIDs.add(request.postDataJSON()?.requestId); }
  });
  await context.route(/\/api\/sync\/snapshots\/[^/]+\/pages\/\d+$/, route => {
    const index = Number(new URL(route.request().url()).pathname.split("/").at(-1));
    pageReads.set(index, (pageReads.get(index) || 0) + 1);
    if (index === 1 && blockSecondPage) { blocked++; return route.abort("failed"); }
    return route.continue();
  });
  await context.route(server + "/api/conversations/" + group.id + "/messages", async route => {
    const request = route.request();
    if (request.method() !== "POST" || request.postDataJSON()?.text !== unknownText) return route.continue();
    unknownWrites++;
    unknownClientId = request.postDataJSON().clientMsgId;
    try {
      const headers = await request.allHeaders();
      const status = await new Promise((resolve, reject) => {
        const outgoing = https.request(request.url(), { method: request.method(), headers, agent }, response => {
          response.resume(); response.on("end", () => resolve(response.statusCode));
        });
        outgoing.setTimeout(30000, () => outgoing.destroy(Error("Unknown-send proxy timeout")));
        outgoing.on("error", reject); outgoing.end(request.postDataBuffer());
      });
      assert.equal(status, 200); unknownCommitted = true;
    } catch (error) { proxyFailure = String(error.message); }
    await route.abort("failed");
  });
  const authState = () => page.evaluate(() => JSON.parse(localStorage.getItem("jobsim.auth.active") || "null"));
  try {
    await page.goto(server);
    await page.getByRole("button", { name: "登录", exact: true }).click();
    await page.locator("input[name=username]").fill(a.user.username);
    await page.locator("input[name=password]").fill(password);
    await page.locator(".modal form").getByRole("button", { name: "登录", exact: true }).click();
    await page.locator(".modal-overlay").waitFor({ state: "hidden" });
    await until(() => blocked > 0, "Expected a real multi-page snapshot");
    const auth = await authState(); assert.ok(auth?.deviceId);
    const namespace = server + "|" + boot.deploymentId + "|" + a.user.id + "|personal|" + boot.restoreEpoch + "|" + auth.deviceId;
    const stateKey = "sync-state|" + namespace;
    const [partial] = await readStore(page, [stateKey]);
    assert.ok(partial?.pendingManifest?.pageManifest.length >= 2);
    assert.equal(partial.cursor, undefined);
    assert.equal(await page.locator(".conversation-row").filter({ hasText: title }).count(), 0);
    checks.push("a missing real snapshot page leaves staging intact and commits neither cloud projection nor boundary");

    blockSecondPage = false;
    await page.locator(".conversation-row").filter({ hasText: title }).waitFor({ timeout: 90000 });
    await page.locator(".conversation-row").filter({ hasText: title }).click();
    await page.locator(".message-content").getByText(fixtures[95].text, { exact: true }).waitFor();
    const [installed, installedSpace] = await readStore(page, [stateKey, namespace]);
    assert.ok(installed.cursor && installedSpace.syncRevision);
    assert.match(installed.position, /^(0|[1-9][0-9]*)$/);
    assert.equal(installedSpace.messages[group.id].filter(row => !row.pending).length, 96);
    assert.equal(snapshotIDs.size, 1, "unknown snapshot response must keep its original request ID");
    const initialSnapshotRequestCount = snapshotIDs.size;
    assert.ok(pageReads.get(0) >= 2, "cold staging is verified by fetching every page again");
    checks.push("all original envelopes, final validation and first permission-confirming delta install one complete persistent baseline");
    fs.mkdirSync(out, { recursive: true });
    await page.screenshot({ path: path.join(out, "Web-真实多页同步.png") });

    const peerGroup = await native("/conversations/" + group.id, "GET", undefined, b.token);
    const incoming = "Peer delta " + stamp;
    await native("/conversations/" + group.id + "/messages", "POST", { clientMsgId: crypto.randomUUID(), type: "text", text: incoming, restoreEpoch: boot.restoreEpoch, sendAdmission: peerGroup.sendAdmission }, b.token);
    await page.locator(".message-content").getByText(incoming, { exact: true }).waitFor({ timeout: 60000 });
    const updated = "Edited through sync " + stamp;
    await native("/messages/" + fixtures[0].id, "PATCH", { text: updated, expectedVersion: fixtures[0].version }, a.token);
    await page.locator(".message-content").getByText(updated, { exact: true }).waitFor({ timeout: 60000 });
    const [advanced] = await readStore(page, [stateKey]);
    assert.ok(BigInt(advanced.position) > BigInt(installed.position));
    checks.push("real peer append and author edit advance a continuous durable cursor without legacy history polling");

    await page.locator("textarea").fill(unknownText);
    await page.getByRole("button", { name: "发送", exact: true }).click();
    await until(() => { if (proxyFailure) throw Error(proxyFailure); return unknownCommitted; }, "UI message was not committed before its response was dropped");
    await until(async () => {
      const [queue, space] = await readStore(page, ["outbox|" + namespace, namespace]);
      const accepted = space?.messages?.[group.id]?.filter(row => row.clientMsgId === unknownClientId && !row.pending);
      return unknownClientId && !queue?.some(row => row.clientMsgId === unknownClientId) && accepted?.length === 1;
    }, "Authoritative sync did not reconcile the original unknown Outbox intent");
    assert.equal(unknownWrites, 1);
    checks.push("a real committed UI send with a lost response is reconciled once by its own original client ID without resending");

    const countBeforeReload = snapshots;
    await page.reload();
    await page.locator(".conversation-row").filter({ hasText: title }).waitFor({ timeout: 60000 });
    const [cold] = await readStore(page, [stateKey]);
    assert.equal(cold.storeGeneration, installed.storeGeneration);
    assert.equal(cold.registrationRequestId, installed.registrationRequestId);
    assert.equal(snapshots, countBeforeReload);
    checks.push("a cold UI restores the same physical-store registration and resumes its committed sync cursor");

    const empty = await browser.newContext({ viewport: { width: 1440, height: 960 } });
    await empty.addCookies(await context.cookies(server));
    const emptyPage = await empty.newPage();
    let changedStore = false, emptyWrites = 0;
    emptyPage.on("response", response => { if (response.url() === server + "/api/sync/device" && response.status() === 409) changedStore = true; });
    emptyPage.on("request", request => { if (request.method() === "POST" && /\/api\/conversations\/[^/]+\/messages$/.test(new URL(request.url()).pathname)) emptyWrites++; });
    await emptyPage.goto(server);
    await until(() => changedStore, "An empty physical store must not inherit old registration");
    await emptyPage.locator(".modal form input[name=username]").waitFor();
    assert.equal(emptyWrites, 0);
    await empty.close();
    checks.push("an opaque cookie with a new empty IndexedDB is refused as the old physical store and requests fresh proof without sending");

    await native("/conversations/" + group.id, "PATCH", { ownerId: b.user.id }, a.token);
    const roster = await native("/conversations/" + group.id + "/members", "GET", undefined, b.token);
    const member = roster.items.find(row => row.id === a.user.id); assert.ok(member?.membershipGeneration);
    await native("/conversations/" + group.id + "/members/" + a.user.id + "?membershipGeneration=" + encodeURIComponent(member.membershipGeneration), "DELETE", undefined, b.token);
    await until(async () => {
      const [state, space] = await readStore(page, [stateKey, namespace]);
      return state?.cursor && !state.restriction && !space.conversations.some(row => row.id === group.id) && state.permissionGeneration !== advanced.permissionGeneration;
    }, "Revoked cloud scope did not establish a new restricted baseline");
    const [retained] = await readStore(page, ["sync-retained|" + namespace]);
    const retainedSpace = retained?.space || retained;
    assert.ok(retainedSpace?.messages?.[group.id]?.some(row => row.id === fixtures[1].id && row.text === fixtures[1].text));
    checks.push("membership revocation restricts cloud projections while preserving a previously lawful ordinary local copy outside the visible cloud view");

    await native("/messages/" + fixtures[1].id, "DELETE", undefined, b.token);
    await until(async () => {
      const [holder] = await readStore(page, ["sync-retained|" + namespace]);
      const space = holder?.space || holder;
      const row = space?.messages?.[group.id]?.find(row => row.id === fixtures[1].id);
      return row?.deleted && row.text === "" && !row.attachmentId && !row.attachmentURL;
    }, "Historical legal holder did not clear a later global tombstone");
    checks.push("a later authoritative global tombstone reaches a former member and erases its retained body and media sidecars");

    const storage = await page.evaluate(() => new Promise((resolve, reject) => {
      const open = indexedDB.open("JobsIM", 1);
      open.onsuccess = () => {
        const db = open.result, read = db.transaction("spaces").objectStore("spaces").getAll();
        read.onsuccess = () => { resolve(JSON.stringify({ values: read.result, local: { ...localStorage }, tab: { ...sessionStorage } })); db.close(); };
        read.onerror = () => { reject(read.error); db.close(); };
      };
      open.onerror = () => reject(open.error);
    }));
    for (const secret of [password, a.token, a.refreshToken, b.token, b.refreshToken]) if (secret) assert.equal(storage.includes(secret), false);
    assert.equal(historyReads, 0);
    assert.equal(errors.length, 0, errors.join("\n"));
    checks.push("JavaScript stores contain no credentials and authoritative sync never falls back to legacy history GETs");
    fs.mkdirSync(out, { recursive: true });
    await page.screenshot({ path: path.join(out, "Web-同步撤权与保留副本.png") });
    fs.writeFileSync(path.join(out, "真实同步结果.json"), JSON.stringify({ realBackend: true, productionDist: true, strictNodeCAAndBrowserSPKIPin: true, checks, errors, snapshots, initialSnapshotRequestCount, totalSnapshotRequestIDs: snapshotIDs.size, fixtureMessages: fixtures.length, unknownMessagePosts: unknownWrites, legacyHistoryGets: historyReads, date: new Date().toISOString() }, null, 2));
    console.log(JSON.stringify({ status: "passed", groups: checks.length, report: out }));
  } catch (error) {
    fs.mkdirSync(out, { recursive: true });
    await page.screenshot({ path: path.join(out, "未验收-失败诊断.png") }).catch(() => {});
    fs.writeFileSync(path.join(out, "未验收-失败摘要.json"), JSON.stringify({ checks, errors, message: String(error.message), date: new Date().toISOString() }, null, 2));
    throw error;
  } finally { await browser.close(); agent.destroy(); }
})().catch(error => { console.error(error); process.exitCode = 1; });
