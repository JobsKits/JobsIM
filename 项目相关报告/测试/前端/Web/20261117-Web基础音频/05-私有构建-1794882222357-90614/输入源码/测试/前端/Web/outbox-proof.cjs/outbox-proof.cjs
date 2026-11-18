const fs = require("node:fs"), path = require("node:path"), assert = require("node:assert/strict"), crypto = require("node:crypto");
const { chromium } = require(process.env.JOBSIM_PLAYWRIGHT_MODULE || "playwright");
const root = path.resolve(__dirname, "../../../..");
(async () => {
  const browser = await chromium.launch({ headless: true, executablePath: process.env.BROWSER_EXECUTABLE || "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" });
  const context = await browser.newContext(), page = await context.newPage(), errors = [];
  page.on("pageerror", error => errors.push(error.message));
  await context.route("**/api/**", route => route.abort());
  try {
    await page.goto(process.env.JOBSIM_WEB_URL || "http://127.0.0.1:5189");
    const checks = await page.evaluate(async () => {
      const storage = await import("/Src/storage.ts"), { messagePayload } = await import("/Src/forward.ts"), { emptySpace } = await import("/Src/types.ts");
      const id = () => crypto.randomUUID(), checks = [], principal = id(), deployment = id();
      const make = () => ({ id: id(), conversationId: id(), senderId: principal, clientMsgId: id(), type: "text", text: "original intent", createdAt: new Date().toISOString(), updatedAt: new Date().toISOString(), version: "1", sequence: "0", deleted: false, reactions: [], pinned: false, favorite: false, pending: true, sendAttempted: true, sendAdmission: "original-admission", restoreEpoch: "1" });
      const namespace = modern => [location.origin, deployment, principal, "personal", ...(modern ? ["1"] : []), id()].join("|");
      const scope = [location.origin, deployment, principal, "personal"].join("|");
      const seed = async (key, message) => storage.queueMessage(key, { ...emptySpace(), outbox: [message], messages: { [message.conversationId]: [message] } }, message);
      const proof = message => ({ clientMsgId: message.clientMsgId, conversationId: message.conversationId, originalBody: messagePayload(message) });
      const equal = (actual, expected) => { if (JSON.stringify(actual) !== JSON.stringify(expected)) throw Error("Unexpected result"); };
      const reject = async (operation, expected) => { let refused = false; try { await operation(); } catch (error) { refused = true; if (expected && !expected(error)) throw Error("Wrong refusal reason"); } if (!refused) throw Error("Expected a refusal"); };
      let key = namespace(false), original = make(); await seed(key, original);
      equal(await storage.acknowledgeOriginalOutbox(key, principal, [proof(original)], () => true), [original.clientMsgId]);
      equal((await storage.loadSpace(key)).outbox.length, 0);
      checks.push("legacy namespace removes only an exact original route and body after a supplied successful read proof");

      for (const change of [message => ({ ...message, text: "different body" }), message => ({ ...message, sendAdmission: "new admission" }), message => ({ ...message, conversationId: id() })]) {
        key = namespace(false); original = make(); await seed(key, original);
        equal(await storage.acknowledgeOriginalOutbox(key, principal, [proof(change(original))], () => true), []);
        equal((await storage.loadSpace(key)).outbox[0].text, original.text);
      }
      checks.push("matching client ID with a changed body, admission or target route never acknowledges the original intent");

      key = namespace(false); original = make(); const before = proof(original); await seed(key, { ...original, text: "new local value" });
      equal(await storage.acknowledgeOriginalOutbox(key, principal, [before], () => true), []);
      equal((await storage.loadSpace(key)).outbox[0].text, "new local value");
      checks.push("the transaction compares the latest durable queue instead of trusting an earlier proof snapshot");

      key = namespace(false); original = { ...make(), senderId: id() }; await seed(key, original);
      equal(await storage.acknowledgeOriginalOutbox(key, principal, [proof(original)], () => true), []);
      equal((await storage.loadSpace(key)).outbox.length, 1);
      checks.push("another sender cannot acknowledge the current principal's queue even with an equal client ID and body");

      key = namespace(true); original = make(); await seed(key, original);
      await reject(() => storage.acknowledgeOriginalOutbox(key, principal, [proof(original)], () => true));
      equal((await storage.loadSpace(key)).outbox.length, 1);
      await storage.isolateAuthNamespace(scope, key);
      equal(await storage.acknowledgeOriginalOutbox(key, principal, [proof(original)], () => true), [original.clientMsgId]);
      checks.push("a modern namespace requires its durable active-device pointer before exact proof acknowledgment");

      key = namespace(true); original = make(); await storage.isolateAuthNamespace(scope, key); await seed(key, original);
      await storage.isolateAuthNamespace(scope, namespace(true));
      await reject(() => storage.acknowledgeOriginalOutbox(key, principal, [proof(original)], () => true));
      equal((await storage.loadSpace(key)).outbox.length, 1);
      checks.push("an old device cannot erase its frozen queue after another device owns the scope");

      key = namespace(false); original = make(); await storage.isolateAuthNamespace(scope, key); await seed(key, original);
      await reject(() => storage.acknowledgeOriginalOutbox(key, principal, [proof(original)], () => false));
      equal((await storage.loadSpace(key)).outbox.length, 1);
      checks.push("a late authentication lifecycle refuses the transaction without erasing any intent");

      key = namespace(false); original = make(); await storage.isolateAuthNamespace(scope, key); await seed(key, original);
      const malformed = proof(original); malformed.originalBody.loop = malformed.originalBody;
      await reject(() => storage.acknowledgeOriginalOutbox(key, principal, [malformed], () => true), error => error instanceof TypeError && /circular|cyclic/i.test(error.message));
      equal((await storage.loadSpace(key)).outbox.length, 1);
      checks.push("a malformed proof aborts the entire IndexedDB transaction without an unhandled page error");

      const { materializeSyncSpace, validOriginalMessageResult } = await import("/Src/SyncView.ts");
      const conversation = id(), serverID = id(), opaque = id().replaceAll("-", "");
      const ref = (owner, kind, value) => ({ owner, kind, id: value, revision: "1", revisionEpoch: "1", state: "active" });
      const base = { ref: ref("conversation", "conversation", conversation), state: "available", revisionMatch: true, object: { id: conversation, kind: "group", title: "Cross-platform", ownerId: principal, memberIds: [principal], updatedAt: new Date().toISOString(), cursor: "0" } };
      const incoming = { ref: { ...ref("message", "message", serverID), conversationId: conversation }, state: "available", revisionMatch: true, object: { ...make(), id: serverID, conversationId: conversation, clientMsgId: opaque } };
      equal(materializeSyncSpace(emptySpace(), [base, incoming], { replaceCloud: true }).messages[conversation][0].clientMsgId, opaque);
      incoming.object.clientMsgId = "legacy-client-key";
      equal(materializeSyncSpace(emptySpace(), [base, incoming], { replaceCloud: true }).messages[conversation][0].clientMsgId, "legacy-client-key");
      incoming.object.clientMsgId = "😀".repeat(40);
      let invalid = false; try { materializeSyncSpace(emptySpace(), [base, incoming], { replaceCloud: true }); } catch { invalid = true; }
      if (!invalid) throw Error("Client ID byte limit was ignored");
      checks.push("valid opaque CLI client IDs are accepted without a UUID assumption and the server UTF-8 byte limit is enforced");

      const result = { ...make(), id: serverID, conversationId: conversation, clientMsgId: opaque, contentVersion: "1", sequence: "5", pending: false };
      equal(validOriginalMessageResult(result, principal, conversation, opaque), true);
      equal(validOriginalMessageResult({ senderId: principal, conversationId: conversation, clientMsgId: opaque }, principal, conversation, opaque), false);
      equal(validOriginalMessageResult({ ...result, id: "not-an-object-id" }, principal, conversation, opaque), false);
      equal(validOriginalMessageResult({ ...result, version: 1 }, principal, conversation, opaque), false);
      equal(validOriginalMessageResult({ ...result, clientMsgId: id() }, principal, conversation, opaque), false);
      checks.push("a malformed 200 result with only matching attribution fields cannot create an original-body acknowledgment proof");
      return checks;
    });
    assert.equal(errors.length, 0, errors.join("\n"));
    const out = process.env.JOBSIM_OUTBOX_REPORT || path.join(root, "项目相关报告/测试/前端/Web/执行记录", "outbox-proof-" + Date.now() + "-" + process.pid);
    fs.mkdirSync(out, { recursive: true });
    const inputs = ["代码/前端/Web/Src/storage.ts", "代码/前端/Web/Src/SyncView.ts", "代码/前端/Web/Src/forward.ts", "测试/前端/Web/outbox-proof.cjs/outbox-proof.cjs"];
    const hashes = Object.fromEntries(inputs.map(file => [file, crypto.createHash("sha256").update(fs.readFileSync(path.join(root, file))).digest("hex")]));
    fs.writeFileSync(path.join(out, "原请求队列结果.json"), JSON.stringify({ runtime: "real Chrome IndexedDB; all API traffic blocked; supplied proofs are test doubles", checks, errors, hashes, date: new Date().toISOString() }, null, 2));
    console.log(JSON.stringify({ status: "passed", groups: checks.length, report: out }));
  } finally { await browser.close(); }
})().catch(error => { console.error(error); process.exitCode = 1; });
