const fs = require("node:fs");
const path = require("node:path");
const crypto = require("node:crypto");
const assert = require("node:assert/strict");
const { pathToFileURL } = require("node:url");

async function main() {
  const root = path.resolve(__dirname, "../../../..");
  const inputs = ["代码/前端/Web/Src/AccountDeletionCleanup.ts", "代码/前端/Web/Src/AccountDeletionStore.ts", "代码/前端/Web/Src/storage.ts", path.relative(root, __filename)];
  const hashes = () => Object.fromEntries(inputs.map(file => [file, crypto.createHash("sha256").update(fs.readFileSync(path.join(root, file))).digest("hex")]));
  const before = hashes(), checks = [];
  const { completedErasureScope, completedErasureKey, completedErasureKeyForNamespace } = await import(pathToFileURL(path.join(root, inputs[0])).href);
  const id = () => crypto.randomUUID(), origin = "https://jobsim.test", scopeId = id(), userId = id(), deviceId = id();
  const identity = { scopeId, userId, deviceId, familyId: id(), epoch: "7", username: "synthetic-owner", namespace: [origin, scopeId, userId, "personal", "7", deviceId].join("|") };
  const scope = [origin, scopeId, userId, "personal"].join("|");
  const check = (name, test) => { test(); checks.push(name); };
  check("erasure requires the exact original deployment/user/device/epoch namespace", () => {
    assert.equal(completedErasureScope(identity, origin), scope);
    for (const field of ["scopeId", "userId", "deviceId", "familyId"]) assert.throws(() => completedErasureScope({ ...identity, [field]: "" }, origin));
    assert.throws(() => completedErasureScope({ ...identity, namespace: identity.namespace.replace(userId, id()) }, origin));
    assert.throws(() => completedErasureScope(identity, "https://another-origin.test"));
  });
  check("numeric, zero, padded and overflowing epochs do not authorize cleanup", () => {
    for (const epoch of [7, "0", "01", "9223372036854775808", undefined]) assert.throws(() => completedErasureScope({ ...identity, epoch }, origin));
  });
  check("boxed identifiers cannot authorize deletion by implicit string coercion", () => {
    for (const field of ["scopeId", "userId", "deviceId", "familyId", "namespace"])
      assert.throws(() => completedErasureScope({ ...identity, [field]: new String(identity[field]) }, origin));
  });
  const namespaces = [identity.namespace, [origin, scopeId, userId, "personal", "2", id()].join("|"), [origin, scopeId, userId, "personal", id()].join("|")];
  check("all original devices and epochs, including legacy and retained archives, belong to the same personal account", () => {
    for (const ns of namespaces) for (const key of [ns, ns + "|archive|9", "sync-retained|" + ns + "|archive|9"]) assert.ok(completedErasureKey(key, scope), key);
  });
  check("every maintained payload holder, including Blob queues and failed-cleanup markers, is in the erasure inventory", () => {
    for (const prefix of ["outbox|", "sync-state|", "sync-retained|", "sync-personal|", "self-state-operations|", "data-exports|", "playback-checkpoints|", "migration-history|", "attachment-sends|", "attachment-cleanup-pending|", "receipt-queries|"])
      assert.equal(completedErasureKey(prefix + identity.namespace, scope)?.namespace, identity.namespace, prefix);
  });
  check("snapshot, schema and migration stages require an exact namespace and an actual UUID stage", () => {
    for (const prefix of ["sync-stage|", "sync-schema-archive|", "migration-stage|"]) {
      assert.ok(completedErasureKey(prefix + identity.namespace + "|" + id(), scope));
      assert.equal(completedErasureKey(prefix + identity.namespace + "|not-a-stage", scope), undefined);
    }
  });
  check("original operation suffixes never change the namespace owner", () => {
    assert.ok(completedErasureKey("operation|" + identity.namespace + "|avatar|original", scope));
    assert.equal(completedErasureKey("operation|" + identity.namespace, scope), undefined);
    assert.equal(completedErasureKey("operation|" + identity.namespace.replace(userId, id()) + "|original", scope), undefined);
  });
  check("other users, deployments, origins and nonpersonal sources remain separate", () => {
    for (const ns of [identity.namespace.replace(userId, id()), identity.namespace.replace(scopeId, id()), identity.namespace.replace(origin, "https://other.test"), identity.namespace.replace("|personal|", "|service|")]) {
      assert.equal(completedErasureKey(ns, scope), undefined);
      assert.equal(completedErasureKey("attachment-sends|" + ns, scope), undefined);
    }
  });
  check("data-rights receipts, shared authentication history and global facts are never blanket-deleted", () => {
    for (const key of ["account-deletions|" + origin + "|" + scopeId, "password-changes|" + origin + "|" + scopeId, "global-facts|" + scope, "auth-active|" + scope, "unknown|" + identity.namespace]) assert.equal(completedErasureKey(key, scope), undefined);
  });
  check("receipt query responsibilities are erased by exact account ownership across devices and epochs", () => {
    for (const ns of namespaces) {
      assert.ok(completedErasureKey("receipt-queries|" + ns, scope));
      assert.ok(completedErasureKey("receipt-queries|" + ns + "|archive|9", scope));
    }
    for (const ns of [identity.namespace.replace(userId, id()), identity.namespace.replace(scopeId, id()), identity.namespace.replace(origin, "https://other.test")])
      assert.equal(completedErasureKey("receipt-queries|" + ns, scope), undefined);
    assert.equal(completedErasureKey("receipt-queries|" + identity.namespace + "|unexpected", scope), undefined);
  });
  check("the original auth fence is retained while payload holders are removed", () => {
    assert.equal(completedErasureKey("auth-frozen|" + identity.namespace, scope)?.fence, true);
    assert.equal(completedErasureKey("attachment-sends|" + identity.namespace, scope)?.fence, false);
  });
  check("array keys, invalid namespaces and owner-like substrings cannot trigger deletion", () => {
    for (const key of [null, 1, [identity.namespace], "prefix" + identity.namespace, identity.namespace + "|unexpected", "attachment-sends|" + identity.namespace + "|unexpected", identity.namespace.replace("|7|", "|01|")]) assert.equal(completedErasureKey(key, scope), undefined);
  });
  check("the permanent write barrier is shared by old epochs and retained archives", () => {
    for (const ns of namespaces) {
      assert.equal(completedErasureKeyForNamespace(ns), "account-erasure|" + scope);
      assert.equal(completedErasureKeyForNamespace(ns + "|archive|9"), "account-erasure|" + scope);
    }
    assert.equal(completedErasureKeyForNamespace("demo|synthetic"), undefined);
  });
  check("a matching username alone cannot establish original local ownership", () => {
    assert.throws(() => completedErasureScope({ username: identity.username }, origin));
    const foreign = { ...identity, userId: id() };
    assert.throws(() => completedErasureScope(foreign, origin));
  });
  const after = hashes();
  assert.deepEqual(after, before);
  const out = path.join(root, "项目相关报告/测试/前端/Web/20261117-注销本机清理/规则-" + Date.now());
  fs.mkdirSync(out, { recursive: true });
  const result = { passed: checks.length, checks, sourceBefore: before, sourceAfter: after, indexedDB: false, browser: false, typeCheck: false, scope: "actual production key/ownership rules only; no claim of transaction, erasure or UI execution" };
  fs.writeFileSync(path.join(out, "结果.json"), JSON.stringify(result, null, 2) + "\n");
  console.log(JSON.stringify({ passed: checks.length, report: path.relative(root, out), indexedDB: false }));
}
main().catch(error => { console.error(error); process.exitCode = 1; });
