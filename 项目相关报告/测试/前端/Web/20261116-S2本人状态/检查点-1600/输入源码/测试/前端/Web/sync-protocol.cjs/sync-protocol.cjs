const fs = require("node:fs");
const path = require("node:path");
const assert = require("node:assert/strict");
const { createHash, randomUUID } = require("node:crypto");
const { execFileSync } = require("node:child_process");
const os = require("node:os");
const root = path.resolve(__dirname, "../../../..");
const sourceFile = path.join(root, "代码/前端/Web/Src/SyncProtocol.ts");
const source = fs.readFileSync(sourceFile, "utf8");
const scratch = fs.mkdtempSync(path.join(os.tmpdir(), "JobsIM-SyncProtocol-"));
let protocol;
try {
  execFileSync(process.execPath, [path.join(root, "代码/前端/Web/node_modules/typescript/bin/tsc"), sourceFile, "--ignoreConfig", "--target", "ES2022", "--module", "CommonJS", "--lib", "ES2022,DOM", "--strict", "--skipLibCheck", "--outDir", scratch], { cwd: scratch, stdio: "pipe" });
  protocol = require(path.join(scratch, "SyncProtocol.js"));
} finally {
  for (const entry of fs.readdirSync(scratch, { withFileTypes: true })) {
    if (!entry.isFile() || !entry.name.endsWith(".js")) throw Error("Unexpected compiler output retained in " + scratch);
    fs.unlinkSync(path.join(scratch, entry.name));
  }
  fs.rmdirSync(scratch);
}
const hash = value => createHash("sha256").update(value).digest("hex");
const envelope = value => {
  const bytes = Buffer.from(JSON.stringify(value));
  return { encoded: bytes.toString("base64url"), digest: hash(bytes) };
};

(async () => {
  const checks = [];
  const check = async (name, run) => { await run(); checks.push(name); };
  const binding = { scopeId: randomUUID(), principalId: randomUUID(), sourceContextId: "personal", deviceId: randomUUID(), storeGeneration: randomUUID(), restoreEpoch: "1", streamId: randomUUID() };
  const ref = { owner: "message", kind: "message", id: randomUUID(), revision: "9007199254740997", revisionEpoch: "1", subjectUserId: binding.principalId, conversationId: randomUUID(), state: "active" };
  const body = { id: ref.id, text: "<tag>&\u2028藏文 བོད་", version: "9007199254740997" };
  const bytes = envelope([ref]), bodyBytes = envelope(body);
  const wire = {
    ...binding, snapshotId: randomUUID(), schemaVersion: "1", boundary: "9007199254740997", boundaryToken: "opaque.signed-token",
    ownerWatermarks: { auth: "3", directory: "8", conversation: "12", message: ref.revision },
    permissionGeneration: "a".repeat(64), classificationWatermark: ref.revision, coverage: { included: ["authorized_messages_and_interactions"], excluded: ["qualified_ack"] },
    pageManifest: [{ page: 0, count: 1, digest: bytes.digest }], expiresAt: new Date(Date.now() + 600000).toISOString(),
  };
  const manifest = protocol.parseSyncManifest(wire, binding);
  const projection = { ref, state: "available", currentRevision: ref.revision, currentRevisionEpoch: "1", revisionMatch: true, bodyDigest: bodyBytes.digest, bodyEnvelope: bodyBytes.encoded, object: body };
  const page = { snapshotId: wire.snapshotId, page: 0, refs: [ref], referenceEnvelope: bytes.encoded, pageDigest: bytes.digest, items: [projection], permissionGeneration: wire.permissionGeneration, restoreEpoch: "1", digestScope: "immutable_references_only", installable: true, requiresFinalValidation: true, requiresAtomicInstall: true, qualifiedAck: false };
  const validation = { canInstall: true, boundaryToken: wire.boundaryToken, restoreEpoch: "1", permissionGeneration: wire.permissionGeneration, classificationWatermark: wire.classificationWatermark, requiresLocalAtomicInstall: true, qualifiedAck: false };
  const expectFailure = (run, code) => assert.rejects(run, error => error instanceof protocol.SyncProtocolError && (!code || error.code === code));
  await check("exact positions beyond JavaScript safe integer remain strings", async () => {
    assert.equal(manifest.boundary, "9007199254740997");
    for (const bad of [9007199254740997, "01", "-1", "9223372036854775808"]) assert.throws(() => protocol.syncCounter(bad));
  });
  await check("snapshot identity, complete ordered manifest and expiry are required", async () => {
    assert.throws(() => protocol.parseSyncManifest(wire, { ...binding, deviceId: randomUUID() }));
    assert.throws(() => protocol.parseSyncManifest({ ...wire, pageManifest: [{ ...wire.pageManifest[0], page: 1 }] }, binding));
    assert.throws(() => protocol.parseSyncManifest({ ...wire, expiresAt: "2001-01-01T00:00:00Z" }, binding));
  });
  await check("original Go-style escaped body bytes are hashed before decoding", async () => {
    const raw = Buffer.from(JSON.stringify(body).replace(/</g, "\\u003c").replace(/>/g, "\\u003e").replace(/&/g, "\\u0026").replace(/\u2028/g, "\\u2028"));
    const parsed = await protocol.parseSyncPage({ ...page, items: [{ ...projection, bodyDigest: hash(raw), bodyEnvelope: raw.toString("base64url") }] }, manifest, 0);
    assert.deepEqual(parsed.items[0].object, body);
  });
  await check("valid digest cannot authorize a different refs array or body", async () => {
    await expectFailure(() => protocol.parseSyncPage({ ...page, refs: [{ ...ref, id: randomUUID() }] }, manifest, 0));
    await expectFailure(() => protocol.parseSyncPage({ ...page, items: [{ ...projection, object: { ...body, text: "changed" } }] }, manifest, 0));
    await expectFailure(() => protocol.parseSyncPage({ ...page, referenceEnvelope: envelope([{ ...ref, id: randomUUID() }]).encoded }, manifest, 0), "SYNC_DIGEST_MISMATCH");
  });
  await check("installable flag cannot override missing or contradictory revision binding", async () => {
    const noRevision = { ...projection }; delete noRevision.currentRevision;
    await expectFailure(() => protocol.parseSyncPage({ ...page, items: [noRevision] }, manifest, 0));
    await expectFailure(() => protocol.parseSyncPage({ ...page, items: [{ ...projection, currentRevision: "4" }] }, manifest, 0), "SYNC_REVISION_MISMATCH");
    await expectFailure(() => protocol.parseSyncPage({ ...page, items: [{ ...projection, revisionMatch: false }] }, manifest, 0), "SYNC_REVISION_MISMATCH");
  });
  await check("unknown, redacted and incomplete pages cannot become a baseline", async () => {
    await expectFailure(() => protocol.parseSyncPage({ ...page, items: [] }, manifest, 0));
    await expectFailure(() => protocol.parseSyncPage({ ...page, installable: false }, manifest, 0), "SYNC_PAGE_NOT_INSTALLABLE");
    await expectFailure(() => protocol.parseSyncPage({ ...page, items: [{ ...projection, state: "unknown" }] }, manifest, 0));
  });
  await check("unrelated append may advance classification watermark without invalidating refs", async () => {
    assert.equal(protocol.checkSyncValidation({ ...validation, classificationWatermark: "9007199254741000" }, manifest), "9007199254741000");
    assert.throws(() => protocol.checkSyncValidation({ ...validation, classificationWatermark: "1" }, manifest));
    assert.throws(() => protocol.checkSyncValidation({ ...validation, permissionGeneration: "b".repeat(64) }, manifest));
  });
  const event = { kind: "reference", position: "11", originEventId: randomUUID(), ref, projection };
  const eventBytes = envelope({ position: event.position, originEventId: event.originEventId, ref });
  event.eventEnvelope = eventBytes.encoded; event.eventDigest = eventBytes.digest;
  const delta = { items: [event, { kind: "redacted", from: "12", to: "14", rangeDigest: "c".repeat(64) }], cursor: "next.opaque", position: "14", head: "14", hasMore: false, restoreEpoch: "1", permissionGeneration: wire.permissionGeneration, snapshotRequired: false, qualifiedAck: false };
  await check("explicit redacted range advances only the continuous boundary", async () => {
    const result = await protocol.parseSyncChanges(delta, "10", "1", wire.permissionGeneration);
    assert.equal(result.position, "14"); assert.equal(result.items[1].kind, "redacted");
    assert.equal("ref" in result.items[1], false);
  });
  await check("gaps, duplicate origins and a false end position stop cursor advancement", async () => {
    await expectFailure(() => protocol.parseSyncChanges({ ...delta, items: [{ ...delta.items[1], from: "11" }] }, "9", "1", wire.permissionGeneration), "SYNC_CONTINUITY_GAP");
    await expectFailure(() => protocol.parseSyncChanges({ ...delta, items: [event, { ...event, position: "12" }] }, "10", "1", wire.permissionGeneration), "SYNC_CONTINUITY_GAP");
    await expectFailure(() => protocol.parseSyncChanges({ ...delta, position: "15" }, "10", "1", wire.permissionGeneration), "SYNC_CONTINUITY_GAP");
  });
  await check("permission and restore changes stop all ordinary delta installation", async () => {
    await expectFailure(() => protocol.parseSyncChanges({ ...delta, permissionGeneration: "b".repeat(64) }, "10", "1", wire.permissionGeneration), "SYNC_PERMISSION_CHANGED");
    await expectFailure(() => protocol.parseSyncChanges({ ...delta, snapshotRequired: true }, "10", "1", wire.permissionGeneration), "SYNC_PERMISSION_CHANGED");
    await expectFailure(() => protocol.parseSyncChanges({ ...delta, restoreEpoch: "2" }, "10", "1", wire.permissionGeneration), "RESTORE_EPOCH_CHANGED");
  });
  await check("delta may hydrate a newer current revision without changing immutable origin", async () => {
    const newer = { ...event, projection: { ...projection, currentRevision: "9007199254740998", revisionMatch: false } };
    const result = await protocol.parseSyncChanges({ ...delta, items: [newer], position: "11", head: "11" }, "10", "1", wire.permissionGeneration);
    assert.equal(result.items[0].projection.currentRevision, "9007199254740998");
  });
  await check("unknown hydration cannot silently advance past an unresolved event", async () => {
    const unknown = { ...event, projection: { ref, state: "unknown", revisionMatch: false } };
    await expectFailure(() => protocol.parseSyncChanges({ ...delta, items: [unknown], position: "11", head: "11" }, "10", "1", wire.permissionGeneration), "SYNC_PROJECTION_UNKNOWN");
  });
  const out = process.env.JOBSIM_SYNC_REPORT || path.join(root, "项目相关报告/测试/前端/Web/执行记录", "sync-protocol-" + Date.now() + "-" + process.pid);
  fs.mkdirSync(out, { recursive: true });
  const result = { runtime: "Node WebCrypto and TypeScript transpile; no browser, API or device", checks, errors: [], sourceSHA256: hash(source), date: new Date().toISOString() };
  fs.writeFileSync(path.join(out, "协议结果.json"), JSON.stringify(result, null, 2));
  console.log(JSON.stringify({ status: "passed", groups: checks.length, report: out }));
})().catch(error => { console.error(error); process.exitCode = 1; });
