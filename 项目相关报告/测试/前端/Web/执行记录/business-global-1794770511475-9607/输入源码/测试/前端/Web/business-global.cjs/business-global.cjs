const fs = require("node:fs"), path = require("node:path"), os = require("node:os"), crypto = require("node:crypto"), assert = require("node:assert/strict");
const { chromium } = require(process.env.JOBSIM_PLAYWRIGHT_MODULE || path.join(os.homedir(), ".cache/codex-runtimes/codex-primary-runtime/dependencies/node/node_modules/playwright"));
const root = path.resolve(__dirname, "../../../.."), origin = process.env.JOBSIM_WEB_URL || "http://127.0.0.1:5189";
const out = path.join(root, "项目相关报告/测试/前端/Web/执行记录", "business-global-" + Date.now() + "-" + process.pid);
fs.mkdirSync(out, { recursive: true });
const inputs = ["MessageFacts.ts", "GlobalMessages.ts", "StorageDatabase.ts", "api.ts", "AuthSession.ts", "storage.ts", "SyncStore.ts", "SyncView.ts", "SyncProtocol.ts", "forward.ts", "types.ts", "App.tsx"].map(file => "代码/前端/Web/Src/" + file);
inputs.push(path.relative(root, __filename));
const hashes = Object.fromEntries(inputs.map(file => {
  const bytes = fs.readFileSync(path.join(root, file)), target = path.join(out, "输入源码", file);
  fs.mkdirSync(path.dirname(target), { recursive: true }); fs.writeFileSync(target, bytes);
  return [file, crypto.createHash("sha256").update(bytes).digest("hex")];
}));
(async () => {
  const browser = await chromium.launch({ headless: true, executablePath: process.env.BROWSER_EXECUTABLE || "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" });
  const watchdog = setTimeout(() => browser.close().catch(() => {}), 60000), errors = [];
  try {
    const context = await browser.newContext(), page = await context.newPage();
    page.on("pageerror", error => errors.push(error.message));
    await context.route("**/api/**", route => route.abort());
    await context.route(origin + "/business-global-test", route => route.fulfill({ contentType: "text/html", body: "<!doctype html><meta charset=utf-8><title>Business message globals</title>" }));
    await page.goto(origin + "/business-global-test");
    const checks = await page.evaluate(async () => {
      const facts = await import("/Src/MessageFacts.ts"), storage = await import("/Src/storage.ts"), globals = await import("/Src/GlobalMessages.ts"), { api } = await import("/Src/api.ts"), { emptySpace } = await import("/Src/types.ts");
      const id = () => crypto.randomUUID(), principal = id(), deployment = id(), device = id(), conversation = id(), checks = [];
      const scope = [location.origin, deployment, principal, "personal"].join("|"), namespace = scope + "|" + device;
      const message = { id: id(), senderId: principal, conversationId: conversation, clientMsgId: id().replaceAll("-", ""), type: "text", text: "ORIGINAL_SOURCE", version: "1", contentVersion: "1", sequence: "1", createdAt: new Date().toISOString(), updatedAt: new Date().toISOString(), deleted: false, reactions: [], pinned: true, favorite: true };
      const forwarded = { ...message, id: id(), forwarded: true, text: "ORIGINAL_FORWARD", forwardAdmission: "TEST_ONLY_PROOF" };
      const original = { ...message, id: id(), pending: true, sendAttempted: true, text: "ORIGINAL_UNKNOWN_INTENT", sendAdmission: "ORIGINAL_ADMISSION" };
      const before = { ...emptySpace(), messages: { [conversation]: [message, forwarded, original] }, outbox: [original] };
      await storage.isolateAuthNamespace(scope, namespace); await storage.saveSpace(namespace, before);
      facts.bindMessageNamespace(namespace);
      let notices = [], responder = async () => ({ ...message });
      const unsubscribe = facts.subscribeGlobalMessages(notice => notices.push(notice));
      const originalFetch = window.fetch;
      window.fetch = async (url, init) => {
        const route = new URL(String(url), location.origin).pathname;
        if (route === "/api/auth/csrf") return Response.json({ csrfToken: "test-csrf" });
        if (route.startsWith("/api/")) return Response.json(await responder(route, init));
        return originalFetch(url, init);
      };
      const check = (condition, reason) => { if (!condition) throw Error(reason); };
      try {
        let release, arrived;
        const entered = new Promise(resolve => { arrived = resolve; });
        responder = async route => route.endsWith(forwarded.id)
          ? new Promise(resolve => { release = resolve; arrived(); }) : { ...message, deleted: true, text: "", version: "2" };
        const late = api("/messages/" + forwarded.id); await entered;
        const deleted = await api("/messages/" + message.id, { method: "DELETE" });
        check(deleted.deleted && !deleted.text && !deleted.favorite && !deleted.pinned, "direct business delete did not apply a global placeholder");
        const durable = await storage.loadSpace(namespace);
        check(durable.messages[conversation].find(row => row.id === message.id).deleted, "global did not commit before API returned");
        check(durable.outbox[0].text === original.text && durable.outbox[0].sendAdmission === original.sendAdmission, "global changed original queue intent");
        check(notices.some(notice => notice.scope === scope && notice.ids.includes(message.id)), "visible holders were not notified");
        checks.push("a complete direct DELETE DTO commits terminal cleanup and notifies visible holders before returning without rewriting Outbox");
        release(forwarded);
        const oldForward = await late;
        check(oldForward.forwardUnavailable && !oldForward.text && !oldForward.forwardAdmission, "late source projection reopened after global");
        checks.push("a forward response started before a new global fact cannot reopen its pre-delete projection");

        responder = async () => ({ ...message, version: "99", deleted: false });
        const stale = await api("/messages/" + message.id);
        check(stale.deleted && !stale.text && !stale.attachmentId, "higher late business response revived a global body");
        responder = async () => ({ items: [{ ...message, deleted: false, version: "99" }], cursor: "", hasMore: false });
        const page = await api("/search?q=original");
        check(page.items[0].deleted && !page.items[0].text, "search revived a known global");
        checks.push("known terminals mask later exact-read and paged search bodies even when their owner version is higher");

        const unknown = id();
        responder = async () => ({ id: unknown, deleted: true });
        await api("/messages/" + unknown);
        check(!(await globals.loadVerifiedGlobals(namespace)).includes(unknown), "malformed tombstone became trusted global");
        const raw = id();
        responder = async () => ({ items: [{ ...message, id: raw, deleted: true }] });
        await api("/sync/changes?cursor=test");
        check(!(await globals.loadVerifiedGlobals(namespace)).includes(raw), "unverified sync envelope triggered business cleanup");
        checks.push("malformed business tombstones and raw sync pages cannot add trusted global facts");

        const priorNotices = notices.length;
        responder = async () => ({ ...message, deleted: true, text: "", version: "2" });
        await api("/messages/" + message.id);
        check(notices.length === priorNotices, "repeated global read advanced projection generation");
        responder = async () => forwarded;
        const fresh = await api("/messages/" + forwarded.id);
        check(!fresh.forwardUnavailable && fresh.text === forwarded.text, "new exact read could not restore an unaffected forward");
        checks.push("repeating a terminal does not keep invalidating projections and a later exact read can restore an unaffected forward");
        return checks;
      } finally { unsubscribe(); facts.bindMessageNamespace(""); window.fetch = originalFetch; }
    });
    assert.equal(errors.length, 0, errors.join("\n"));
    fs.writeFileSync(path.join(out, "业务终态结果.json"), JSON.stringify({ status: "passed", scope: "real Chrome IndexedDB with simulated business API; no real server, no App GUI assertion", checks, errors, hashes, date: new Date().toISOString() }, null, 2));
    console.log(JSON.stringify({ status: "passed", groups: checks.length, report: out }));
  } catch (error) {
    fs.writeFileSync(path.join(out, "失败记录.json"), JSON.stringify({ status: "failed", error: String(error), hashes, date: new Date().toISOString() }, null, 2));
    throw error;
  } finally { clearTimeout(watchdog); await browser.close(); }
})().catch(error => { console.error(error); process.exitCode = 1; });
