// Host injection tests exercise shipped ArkTS and real private SQLite, not Harmony RDB NAPI.
const test = require('node:test');
const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');
const { spawn } = require('node:child_process');
const work = process.env.JOBS_IM_SYNC_HOST_WORK;
const { SyncRules, SyncProtocol } = require(path.join(work, 'SyncProtocol.js'));
const { SyncStore } = require(path.join(work, 'SyncStore.js'));
const { SyncEngine } = require(path.join(work, 'SyncEngine.js'));
const { SelfRules } = require(path.join(work, 'SelfProtocol.js'));
const { SelfState } = require(path.join(work, 'SelfState.js'));
const { SelfText } = require(path.join(work, 'SelfText.js'));
const { ApiFailure } = require(path.join(work, 'Models.js'));
const uuid = () => crypto.randomUUID();
const codec = {
  bytes: text => new Uint8Array(Buffer.from(text, 'utf8')),
  text: bytes => new TextDecoder('utf-8', { fatal: true }).decode(bytes),
  encode: bytes => Buffer.from(bytes).toString('base64url'),
  decode: text => new Uint8Array(Buffer.from(text, 'base64url')),
  sha256: async bytes => crypto.createHash('sha256').update(bytes).digest('hex')
};
const hash = value => crypto.createHash('sha256').update(value).digest('hex');
const envelope = value => ({ bodyEnvelope: Buffer.from(value).toString('base64url'), bodyDigest: hash(value) });

class Bridge {
  constructor(file) {
    this.serial = 0; this.pending = new Map(); this.buffer = ''; this.tail = Promise.resolve();
    this.process = spawn('python3', ['-u', process.env.JOBS_IM_SYNC_SQL_BRIDGE, file], { stdio: ['pipe', 'pipe', 'pipe'] });
    this.errors = ''; this.process.stderr.on('data', data => this.errors += data);
    this.process.stdout.on('data', data => {
      this.buffer += data;
      while (this.buffer.includes('\n')) {
        const at = this.buffer.indexOf('\n'); const row = JSON.parse(this.buffer.slice(0, at)); this.buffer = this.buffer.slice(at + 1);
        const waiter = this.pending.get(row.id); this.pending.delete(row.id);
        if (row.error) waiter.reject(new Error(row.error)); else waiter.resolve(row.rows);
      }
    });
    this.process.on('exit', () => { for (const waiter of this.pending.values()) waiter.reject(new Error('SQLite bridge exited ' + this.errors)); this.pending.clear(); });
  }
  sql(sql, values = []) {
    const result = this.tail.then(() => new Promise((resolve, reject) => {
      const id = ++this.serial; this.pending.set(id, { resolve, reject });
      this.process.stdin.write(JSON.stringify({ id, sql, values }) + '\n');
    }));
    this.tail = result.then(() => {}, () => {}); return result;
  }
  async executeSql(sql, values = []) { await this.sql(sql, values); }
  async querySql(sql, values = []) {
    const rows = await this.sql(sql, values); let at = -1;
    return { goToNextRow: () => ++at < rows.length, getString: column => String(rows[at][column]), close() {} };
  }
  beginTransaction() { this.sql('BEGIN IMMEDIATE'); }
  commit() { this.sql('COMMIT'); }
  rollBack() { this.sql('ROLLBACK'); }
  async close() { await this.tail; this.process.stdin.end(); await new Promise(resolve => this.process.once('exit', resolve)); }
}

async function fixture() {
  const dir = fs.mkdtempSync(path.join(work, 'SQLite-')); const bridge = new Bridge(path.join(dir, 'state.db'));
  const source = fs.readFileSync(process.env.JOBS_IM_SYNC_NATIVE_STORE, 'utf8');
  for (const match of source.matchAll(/executeSql\('(CREATE TABLE IF NOT EXISTS [^']+)'\)/g)) await bridge.executeSql(match[1]);
  let tail = Promise.resolve(); const local = { hooks: undefined,
    setSyncHooks(hooks) { this.hooks = hooks; },
    syncAccess(action) { const task = tail.then(() => action(bridge)); tail = task.then(() => {}, () => {}); return task; }
  };
  const store = new SyncStore(local, codec, uuid); await store.open();
  const identity = { namespace: 'native-' + uuid(), profile: 'strict-test', deployment: uuid(), user: uuid(), source: 'personal', epoch: '1', device: uuid() };
  const conversation = { id: uuid(), kind: 'group', title: 'Native S1', memberIds: [identity.user], membershipGeneration: '1', roleGeneration: '1', sendAdmission: 'original-admission', readOnly: false };
  const message = { id: uuid(), conversationId: conversation.id, senderId: uuid(), clientMsgId: '0123456789abcdef0123456789abcdef', type: 'text', text: 'lawful ordinary history', createdAt: '2026-11-16T00:00:00Z', version: '9007199254740993', contentVersion: '1', sequence: '1', deleted: false, reactions: [] };
  const state = { current: true, permission: 'a'.repeat(64), capabilities: true, stream: uuid(), requests: [], changes: null, mutatePage: undefined, mutateValidate: undefined, beforeChanges: undefined, results: new Map(), cleanupFails: false, cleans: 0 };
  let manifest;
  function ref(row, owner = 'message', kind = 'message') { return { owner, kind, id: row.id, revision: kind === 'message' ? state.ownerRevision || row.version || '1' : row.version || '1', revisionEpoch: identity.epoch, state: 'active', ...(kind === 'message' ? { conversationId: row.conversationId } : {}) }; }
  function projection(row, owner, kind, status = 'available') { const reference = ref(row, owner, kind); const raw = JSON.stringify(row, null, 1); return { ref: reference, state: status, revisionMatch: true, currentRevision: reference.revision, currentRevisionEpoch: identity.epoch, object: row, ...envelope(raw) }; }
  const items = () => [projection(conversation, 'conversation', 'conversation'), projection(message, 'message', 'message')];
  function delta(projections = [], start = '0') {
    let position = start; const values = projections.map(projected => {
      position = SyncRules.next(position); const origin = uuid(); const fact = JSON.stringify({ position, originEventId: origin, ref: projected.ref }, null, 1);
      return { kind: 'reference', position, originEventId: origin, ref: projected.ref, projection: projected, eventEnvelope: Buffer.from(fact).toString('base64url'), eventDigest: hash(fact) };
    });
    return { restoreEpoch: identity.epoch, permissionGeneration: state.permission, snapshotRequired: false, qualifiedAck: false, items: values, position, head: position, cursor: 'signed-' + position, hasMore: false };
  }
  async function request(route, method, body) {
    state.requests.push({ route, method, body });
    if (route === '/bootstrap') return JSON.stringify({ deploymentId: identity.deployment, restoreEpoch: identity.epoch, features: { authoritative_sync: state.capabilities, sync_jetstream: state.capabilities, sync_self_state: state.self === true, cloud_drafts: state.self === true } });
    const control = await store.syncControl(identity);
    const binding = { scopeId: identity.deployment, principalId: identity.user, sourceContextId: 'personal', restoreEpoch: identity.epoch, deviceId: identity.device, storeGeneration: control.physical, streamId: state.stream };
    if (route === '/sync/device') return JSON.stringify({ ...binding, ...(control.schema === '2' ? { schemaVersion: '2' } : {}), qualification: 'transport_only', qualifiedAck: false });
    if (route.startsWith('/sync/context')) return JSON.stringify({ ...binding, qualification: 'transport_only', qualifiedAck: false, afterReceiptEnabled: false, head: '0', permissionGeneration: state.permission, coverage: { schemaVersion: control.schema || '1' } });
    if (route === '/sync/snapshots') {
      if (manifest && manifest.schemaVersion !== (control.schema || '1')) manifest = undefined;
      if (!manifest) manifest = { ...binding, schemaVersion: control.schema || '1', snapshotId: uuid(), boundary: '0', boundaryToken: 'boundary', permissionGeneration: state.permission, classificationWatermark: '5', expiresAt: new Date(Date.now() + 600000).toISOString(), ownerWatermarks: { auth: '0', directory: '1', conversation: '2', message: '5', ...(control.schema === '2' ? { sync: '1' } : {}) }, pageManifest: items().map((item, page) => ({ page, count: 1, digest: hash(JSON.stringify([item.ref], null, 1)) })) };
      return JSON.stringify(manifest);
    }
    if (/\/pages\/\d+$/.test(route)) {
      const index = Number(route.split('/').pop()); const item = items()[index]; const raw = JSON.stringify([item.ref], null, 1);
      const page = { snapshotId: manifest.snapshotId, page: index, pageDigest: manifest.pageManifest[index].digest, permissionGeneration: state.permission, restoreEpoch: identity.epoch, installable: true, requiresFinalValidation: true, requiresAtomicInstall: true, qualifiedAck: false, refs: [item.ref], referenceEnvelope: Buffer.from(raw).toString('base64url'), items: [item] };
      if (state.mutatePage) state.mutatePage(page, index); return JSON.stringify(page);
    }
    if (route.endsWith('/validate')) {
      const result = { canInstall: true, requiresLocalAtomicInstall: true, qualifiedAck: false, restoreEpoch: identity.epoch, permissionGeneration: state.permission, classificationWatermark: '6', boundaryToken: 'boundary' };
      if (state.mutateValidate) state.mutateValidate(result); return JSON.stringify(result);
    }
    if (route.startsWith('/sync/changes?')) { if (state.beforeChanges) await state.beforeChanges(); return JSON.stringify(state.changes || delta()); }
    if (route.endsWith('/message-results')) {
      const id = JSON.parse(body).clientMsgId; const result = state.results.get(id);
      if (result instanceof Error) throw result;
      if (!result) throw new ApiFailure(404, 'MESSAGE_RESULT_NOT_FOUND');
      return JSON.stringify(result);
    }
    throw new Error('Unexpected route ' + route);
  }
  const ports = { store, codec, uuid, current: () => state.current, request,
    cleanup: async () => { const rows = await store.cleanups(); for (const row of rows) { if (state.cleanupFails) throw new Error('cache unlink failed'); state.cleans++; await store.cleaned(row); } } };
  const engine = new SyncEngine(identity, ports);
  async function pending(id = 'fedcba9876543210fedcba9876543210') {
    const row = { ...message, id, clientMsgId: id, senderId: identity.user, sequence: '0', version: '1', state: 'pending', restoreEpoch: identity.epoch, sendAdmission: conversation.sendAdmission, text: 'original intent' };
    await local.syncAccess(async db => { db.beginTransaction(); try { const result = await local.hooks.put(db, identity.namespace, row); await db.executeSql('INSERT OR REPLACE INTO messages VALUES(?,?,?,?,?,?)', [identity.namespace, row.conversationId, row.id, JSON.stringify(result), row.state, row.createdAt]); db.commit(); } catch (error) { db.rollBack(); throw error; } }); return row;
  }
  return { store, local, bridge, identity, conversation, message, state, engine, ports, ref, projection, delta, pending, async rows(sql, args = []) { return bridge.sql(sql, args); }, async close() { await tail; await bridge.close(); } };
}
async function withFixture(action) { const f = await fixture(); try { await action(f); } finally { await f.close(); } }

test('exact counters and opaque cross-end client IDs, explicit full DTO', async () => {
  assert.equal(SyncRules.next('9007199254740993'), '9007199254740994');
  assert.throws(() => SyncRules.counter('9223372036854775808'));
  await withFixture(async f => { SyncRules.message(JSON.stringify(f.message)); assert.throws(() => SyncRules.message(JSON.stringify({ ...f.message, deleted: undefined }))); assert.throws(() => SyncRules.message(JSON.stringify({ ...f.message, contentVersion: undefined }))); });
});
test('raw envelope bytes SHA and structural deep equality both required', async () => {
  const protocol = new SyncProtocol(codec); const raw = '{ "a": 1 }'; assert.equal(await protocol.envelope(Buffer.from(raw).toString('base64url'), hash(raw)), raw);
  await assert.rejects(protocol.envelope(Buffer.from(raw).toString('base64url'), hash('{"a":1}')));
  assert.equal(SyncRules.same(raw, '{"a":1}'), true);
});
test('all pages plus final validation and first changes precede atomic publication', () => withFixture(async f => {
  let reached; const ready = new Promise(resolve => reached = resolve); let release; const gate = new Promise(resolve => release = resolve);
  f.state.beforeChanges = async () => { reached(); await gate; };
  const run = f.engine.run(); await ready;
  assert.equal((await f.rows("SELECT id FROM messages WHERE state='received'")).length, 0);
  assert.equal((await f.rows('SELECT payload FROM sync_stages')).length, 2);
  release(); await run;
  assert.equal((await f.store.syncControl(f.identity)).classification, '6');
  assert.equal((await f.rows("SELECT payload FROM messages WHERE state='received'")).length, 1);
  assert.equal((await f.rows('SELECT payload FROM sync_stages')).length, 0);
}));
test('owner reference revision is independent of Message business version', () => withFixture(async f => {
  f.state.ownerRevision = '7'; await f.engine.run();
  const row = JSON.parse((await f.rows("SELECT payload FROM messages WHERE state='received'"))[0][0]);
  assert.equal(row.version, '9007199254740993');
  assert.equal(JSON.parse((await f.rows("SELECT payload FROM sync_refs WHERE id LIKE 'message:%'"))[0][0]).revision, '7');
}));
for (const [name, mutate] of [
  ['missing page item', page => { page.items = []; }],
  ['raw body hash mismatch', page => { page.items[0].bodyDigest = '0'.repeat(64); }],
  ['outer object mismatch', page => { page.items[0].object = { ...page.items[0].object, title: 'tampered' }; }],
  ['permission ABA', page => { page.permissionGeneration = 'b'.repeat(64); }],
  ['reference current revision mismatch', page => { page.items[0].currentRevision = '2'; }]
]) test(name + ' cannot publish partial body/cursor', () => withFixture(async f => {
  f.state.mutatePage = mutate; await assert.rejects(f.engine.run());
  assert.equal((await f.rows("SELECT id FROM messages WHERE state='received'")).length, 0); assert.equal((await f.store.syncControl(f.identity)).cursor, '');
}));
test('classification watermark regression rejects complete snapshot', () => withFixture(async f => { f.state.mutateValidate = row => row.classificationWatermark = '4'; await assert.rejects(f.engine.run()); assert.equal((await f.store.syncControl(f.identity)).cursor, ''); }));
test('durable original registration and snapshot request survive failed round', () => withFixture(async f => {
  f.state.mutatePage = () => { throw new Error('offline'); }; await assert.rejects(f.engine.run());
  const before = await f.store.syncControl(f.identity); f.state.mutatePage = undefined; await new SyncEngine(f.identity, f.ports).run();
  const requests = f.state.requests.filter(row => row.route === '/sync/snapshots'); assert.equal(requests[0].body, requests[1].body); assert.equal((await f.store.syncControl(f.identity)).physical, before.physical);
}));
test('same client ID from peer never acknowledges own original', () => withFixture(async f => {
  const pending = await f.pending(f.message.clientMsgId); await f.engine.run();
  assert.equal((await f.rows("SELECT id FROM messages WHERE state='pending'")).length, 1); assert.equal((await f.store.syncOriginals(f.identity))[0].id, pending.clientMsgId);
  assert.equal(f.state.requests.filter(row => /\/messages$/.test(row.route)).length, 0);
}));
test('auxiliary 503 and corrupt original evidence are zero proof but verified global still installs', () => withFixture(async f => {
  await f.engine.run(); const pending = await f.pending(); const original = (await f.store.syncOriginals(f.identity))[0]; f.state.results.set(pending.id, new ApiFailure(503, 'OWNER_UNAVAILABLE'));
  const deleted = { id: f.message.id, version: '1', deleted: true }; f.state.changes = f.delta([f.projection(deleted, 'message', 'message', 'global')]);
  f.state.changes.items[0].ref.conversationId = f.conversation.id; f.state.changes.items[0].projection.ref = f.state.changes.items[0].ref;
  const fact = JSON.stringify({ position: '1', originEventId: f.state.changes.items[0].originEventId, ref: f.state.changes.items[0].ref }); Object.assign(f.state.changes.items[0], { eventEnvelope: Buffer.from(fact).toString('base64url'), eventDigest: hash(fact) });
  await f.engine.run(); assert.equal((await f.rows('SELECT id FROM sync_terminal')).length, 1); assert.equal((await f.rows("SELECT id FROM messages WHERE state='received'")).length, 0); assert.equal((await f.store.syncOriginals(f.identity))[0].body, original.body);
  await f.rows("UPDATE sync_originals SET hash='broken' WHERE id=?", [pending.id]); f.state.changes = f.delta([], '1'); await f.engine.run(); assert.equal((await f.rows('SELECT id FROM sync_terminal')).length, 1);
}));
test('full readonly original result confirms only matching sender route/body hash', () => withFixture(async f => {
  await f.engine.run(); const pending = await f.pending(); f.state.results.set(pending.id, { ...f.message, clientMsgId: pending.id, senderId: f.identity.user });
  assert.equal(await f.engine.resolve(pending.id), true); assert.equal((await f.rows("SELECT id FROM messages WHERE state='pending'")).length, 0);
  assert.equal((await f.rows("SELECT state FROM sync_originals WHERE id=?", [pending.id]))[0][0], 'confirmed');
}));
test('malformed complete 200 and wrong sender never confirm queue', () => withFixture(async f => {
  await f.engine.run(); const pending = await f.pending(); f.state.results.set(pending.id, { ...f.message, clientMsgId: pending.id, senderId: f.identity.user, deleted: undefined }); await assert.rejects(f.engine.resolve(pending.id));
  f.state.results.set(pending.id, { ...f.message, clientMsgId: pending.id }); await assert.rejects(f.engine.resolve(pending.id)); assert.equal((await f.store.syncOriginals(f.identity)).length, 1);
}));
test('late context and SQLite transaction failure never publish body/cursor', () => withFixture(async f => {
  f.state.beforeChanges = async () => { f.state.current = false; }; await assert.rejects(f.engine.run()); assert.equal((await f.rows("SELECT id FROM messages WHERE state='received'")).length, 0);
  f.state.current = true; f.state.beforeChanges = undefined;
  await f.rows("CREATE TRIGGER reject_cursor BEFORE INSERT ON local_state WHEN NEW.key='sync:control' AND NEW.value LIKE '%signed-0%' BEGIN SELECT RAISE(ABORT,'disk failure injection'); END");
  await assert.rejects(new SyncEngine(f.identity, f.ports).run()); assert.equal((await f.rows("SELECT id FROM messages WHERE state='received'")).length, 0);
}));
test('ordinary retained copies stay isolated and global dominates newer resurrection', () => withFixture(async f => {
  await f.engine.run(); await f.store.syncRestrict(f.identity, 'SYNC_UNAVAILABLE', false); const rows = await f.store.retained(f.identity, f.conversation.id); assert.equal(rows[0].text, f.message.text);
  const control = await f.store.syncControl(f.identity); const projection = { ref: { ...f.ref(f.message), revision: '1' }, state: 'global', revision: '1', epoch: '1', object: JSON.stringify({ id: f.message.id, version: '1', deleted: true }) };
  await f.store.syncInstall({ expected: control, next: { ...control, guard: uuid(), restriction: '', cursor: 'terminal' }, pages: [], projections: [projection, { ref: f.ref(f.message), state: 'available', revision: f.message.version, epoch: '1', object: JSON.stringify(f.message) }], events: [], proofs: [] }, () => true);
  assert.equal((await f.store.retained(f.identity, f.conversation.id)).length, 0); assert.equal((await f.rows("SELECT id FROM messages WHERE state='received'")).length, 0);
}));
test('forward and announcement source body are never durable projections', () => withFixture(async f => {
  f.message.forwarded = true; f.message.forwardAdmission = 'private-permission'; f.message.attachmentURL = '/api/messages/x/attachment'; await f.engine.run();
  const saved = (await f.rows("SELECT payload FROM messages WHERE state='received'"))[0][0]; assert.equal(JSON.parse(saved).text, ''); assert.equal(saved.includes('private-permission'), false); assert.equal(saved.includes('/attachment'), false);
  const control = await f.store.syncControl(f.identity); const id = uuid(); const announcement = { conversationId: f.conversation.id, message: { text: 'never-store-source' } };
  await f.store.syncInstall({ expected: control, next: { ...control, guard: uuid() }, pages: [], projections: [{ ref: { owner: 'conversation', kind: 'announcement', id, revision: '1', revisionEpoch: '1', state: 'active' }, state: 'available', revision: '1', epoch: '1', object: JSON.stringify(announcement) }], events: [], proofs: [] }, () => true);
  assert.equal((await f.rows("SELECT payload FROM sync_objects WHERE kind='announcement'"))[0][0].includes('never-store-source'), false);
}));
test('terminal cleanup intent survives failure and retries without new events', () => withFixture(async f => {
  await f.engine.run(); const control = await f.store.syncControl(f.identity); f.state.cleanupFails = true;
  await f.store.syncInstall({ expected: control, next: { ...control, guard: uuid() }, pages: [], projections: [{ ref: { ...f.ref(f.message), revision: '1' }, state: 'global', revision: '1', epoch: '1', object: '' }], events: [], proofs: [] }, () => true);
  await assert.rejects(f.ports.cleanup()); assert.ok((await f.store.cleanups()).length > 0); f.state.cleanupFails = false; await new SyncEngine(f.identity, f.ports).run(); assert.equal((await f.store.cleanups()).length, 0);
}));
test('capability absence does not create responsibility; disappearance never legacy falls back', () => withFixture(async f => {
  f.state.capabilities = false; await assert.rejects(f.engine.run()); assert.equal(await f.store.syncControl(f.identity), undefined);
  f.state.capabilities = true; await f.engine.run(); f.state.capabilities = false; await assert.rejects(f.engine.run()); assert.equal((await f.store.syncControl(f.identity)).restriction, 'SYNC_CAPABILITY_REQUIRED');
}));
test('registered physical UUID loss is refused instead of silently regenerating', () => withFixture(async f => {
  await f.engine.run(); await f.rows("DELETE FROM local_state WHERE namespace='__sync_store__' AND key='physical'"); await assert.rejects(f.store.open(), /SYNC_STORE_INVALID/);
}));
test('global cleans all proven same-account old device/epoch holders without crossing other users', () => withFixture(async f => {
  await f.engine.run(); const current = await f.store.syncControl(f.identity);
  const oldIdentity = { ...f.identity, namespace: 'old-device', device: uuid(), epoch: '2' };
  const old = { ...current, identity: oldIdentity, registration: uuid(), guard: uuid() };
  const foreign = { ...old, identity: { ...oldIdentity, namespace: 'foreign', user: uuid() } };
  for (const holder of [old, foreign]) {
    await f.rows('INSERT INTO local_state VALUES(?,?,?)', [holder.identity.namespace, 'sync:control', JSON.stringify(holder)]);
    await f.rows('INSERT INTO sync_retained VALUES(?,?,?,?)', [holder.identity.namespace, f.message.id, f.conversation.id, JSON.stringify(f.message)]);
  }
  await f.store.syncInstall({ expected: current, next: { ...current, guard: uuid() }, pages: [], projections: [{ ref: { ...f.ref(f.message), revision: '1' }, state: 'global', revision: '1', epoch: '1', object: '' }], events: [], proofs: [] }, () => true);
  assert.equal((await f.rows('SELECT namespace FROM sync_retained')).flat().join(','), 'foreign');
  assert.ok((await f.store.cleanups()).some(row => row.namespace === 'old-device'));
}));
test('origin conflict aborts entire projection/cursor transaction', () => withFixture(async f => {
  await f.engine.run(); const control = await f.store.syncControl(f.identity); const origin = uuid(); await f.rows('INSERT INTO sync_events VALUES(?,?,?)', [f.identity.namespace, origin, '{"original":1}']);
  await assert.rejects(f.store.syncInstall({ expected: control, next: { ...control, guard: uuid(), position: '1' }, pages: [], projections: [{ ref: { ...f.ref(f.message), revision: '1' }, state: 'global', revision: '1', epoch: '1', object: '' }], events: [{ origin, fact: '{"different":1}' }], proofs: [] }, () => true), /SYNC_ORIGIN_CONFLICT/);
  assert.equal((await f.rows('SELECT id FROM sync_terminal')).length, 0); assert.equal((await f.store.syncControl(f.identity)).position, '0');
}));
test('proof captured before a guard or payload mutation cannot close the current original', () => withFixture(async f => {
  await f.engine.run(); const pending = await f.pending(); const original = (await f.store.syncOriginals(f.identity))[0]; const control = await f.store.syncControl(f.identity);
  await f.store.syncRestrict(f.identity, 'SYNC_PERMISSION_CHANGED', false);
  await assert.rejects(f.store.syncConfirm(f.identity, control.guard, { original, result: JSON.stringify({ ...f.message, clientMsgId: pending.id, senderId: f.identity.user }) }, () => true), /SYNC_LOCAL_GUARD_CHANGED/);
  assert.equal((await f.store.syncOriginals(f.identity)).length, 1);
}));
test('delta gap and unknown state do not advance a verified old cursor', () => withFixture(async f => {
  await f.engine.run(); f.state.changes = f.delta([f.projection(f.message, 'message', 'message')]); f.state.changes.items[0].position = '2'; await assert.rejects(f.engine.run(), /SYNC_POSITION_GAP/);
  assert.equal((await f.store.syncControl(f.identity)).position, '0');
}));
test('S2 automatic transition archives old stream while retaining physical UUID and original queue', () => withFixture(async f => {
  await f.pending(); await f.engine.run(); const old = await f.store.syncControl(f.identity); const original = (await f.store.syncOriginals(f.identity))[0];
  await f.rows('INSERT INTO sync_events VALUES(?,?,?)',[f.identity.namespace,'prior-origin','{"original":1}']);
  f.state.self = true; f.state.stream = uuid(); await f.engine.run(); const next = await f.store.syncControl(f.identity);
  assert.equal(next.schema,'2'); assert.equal(next.physical,old.physical); assert.notEqual(next.registration,old.registration); assert.notEqual(next.stream,old.stream);
  assert.equal((await f.store.syncOriginals(f.identity))[0].body,original.body); assert.equal((await f.rows("SELECT id FROM sync_stream_archive WHERE kind='sync_events'"))[0][0],'prior-origin');
  await assert.rejects(f.store.syncSelectSchema(next,'1'),/DOWNGRADE/);f.state.self=false;await assert.rejects(f.engine.run(),/CAPABILITY/);
}));
test('S2 ordinary updates cannot replace a schema or registration nonce', () => withFixture(async f => {
  await f.engine.run();const control=await f.store.syncControl(f.identity);
  await assert.rejects(f.store.syncUpdate(control,{...control,schema:'2'}),/TRANSITION/);
  await assert.rejects(f.store.syncUpdate(control,{...control,registration:uuid()}),/CONTEXT/);
}));
test('S2 personal projection has independent object version and exact own identity', () => withFixture(async f => {
  const raw=JSON.stringify({scopeId:f.identity.deployment,sourceContextId:'personal',userId:f.identity.user,revisionEpoch:'1',version:'3',fontPreset:'large',explicit:true});
  const p={ref:{owner:'directory',kind:'preferences',id:f.identity.user,subjectUserId:f.identity.user,revision:'99',revisionEpoch:'1',state:'active'},state:'available',revision:'99',epoch:'1',object:raw};
  assert.equal(SelfRules.projection(p,f.identity,'2'),raw);assert.throws(()=>SelfRules.projection(p,{...f.identity,user:uuid()},'2'),/SCOPE/);assert.throws(()=>SelfRules.projection(p,f.identity,'1'),/SCHEMA/);
}));
test('S2 timestamps accept RFC3339 offsets without rewriting original text', () => {
  SelfRules.timestamp('2026-11-16T12:34:56.123456789+00:00');SelfRules.timestamp('2026-11-16T12:34:56+08:00');assert.throws(()=>SelfRules.timestamp('2026-11-16 12:34:56'));
});
test('S2 default empty draft is valid and cloud projection drops temporary authorizations', () => withFixture(async f => {
  const d={id:f.conversation.id,scopeId:f.identity.deployment,sourceContextId:'personal',userId:f.identity.user,conversationId:f.conversation.id,revisionEpoch:'1',version:'0',contentState:'empty',ownedText:null,replyState:'none',reply:null,draftAdmission:'da1-private',writable:true};
  SelfRules.draft(JSON.stringify(d),f.identity,f.conversation.id);
  const p={ref:{owner:'sync',kind:'draft',id:d.id,subjectUserId:f.identity.user,conversationId:d.conversationId,revision:'77',revisionEpoch:'1',state:'active'},state:'available',revision:'77',epoch:'1',object:JSON.stringify(d)};
  assert.equal(SelfRules.projection(p,f.identity,'2').includes('da1-private'),false);
  assert.throws(()=>SelfRules.draft(JSON.stringify({...d,quote:'unauthorized source body'}),f.identity,f.conversation.id),/INVALID/);
}));
async function selfFixture(f, handler) {
  f.state.self=true;await f.engine.run();
  const calls=[];const service=new SelfState({store:f.local,sync:f.store,identity:f.identity,codec,uuid,current:()=>f.state.current,resync:async()=>{},request:async(route,method,body)=>{calls.push({route,method,body});const handled=await handler(route,method,body);if(handled!==undefined)return JSON.stringify(handled);return f.ports.request(route,method,body)}});
  await service.open();return {service,calls};
}
test('S2 native service preserves prepared body and checks completed current by readonly route', () => withFixture(async f => {
  const operation=uuid();let saved,prepared=false,lookups=0,writes=0;
  const {service}=await selfFixture(f,async(route,method,body)=>{
    if(route.endsWith('/draft-results')){lookups++;if(!prepared)throw new ApiFailure(404,'DRAFT_OPERATION_NOT_FOUND');return {operationId:operation,status:'applied',current:false,result:{revisionEpoch:'1',version:'1',contentState:'owned',replyState:'none'}}}
    if(route.endsWith('/draft')&&method==='PUT'){writes++;assert.equal(body,saved.body);prepared=true;return {operationId:operation,status:'prepared',current:false,result:{revisionEpoch:'1',version:'0',contentState:'empty',replyState:'none'}}}
  });
  const id=uuid();const body=JSON.stringify({requestId:id,revisionEpoch:'1',expectedVersion:'0',draftAdmission:'original-da1',ownedText:'independent input',reply:null});
  saved={id,kind:'draft',conversation:f.conversation.id,epoch:'1',method:'PUT',route:'/conversations/'+f.conversation.id+'/draft',body,hash:'',state:'pending',result:''};
  const guard=await f.store.ready(f.identity);await service.save(saved,guard);
  assert.equal((await service.perform(id,true)).status,'prepared');assert.equal((await service.operations())[0].state,'pending');
  assert.equal((await service.perform(id,true)).current,false);assert.equal((await service.perform(id,true)).current,false);
  assert.equal(writes,1);assert.equal(lookups,3);assert.equal((await service.operations())[0].body,body);
}));
test('S2 completed404 and automatic unknown never create a personal write', () => withFixture(async f => {
  let writes=0;const {service}=await selfFixture(f,async(route,method)=>{if(route.endsWith('-results'))throw new ApiFailure(404,'DRAFT_OPERATION_NOT_FOUND');if(method==='PUT')writes++;});
  const id=uuid();const operation={id,kind:'draft',conversation:f.conversation.id,epoch:'1',method:'PUT',route:'/conversations/'+f.conversation.id+'/draft',body:JSON.stringify({requestId:id,revisionEpoch:'1',expectedVersion:'0',draftAdmission:'da1-old',ownedText:'original',reply:null}),hash:'',state:'pending',result:''};
  await service.save(operation,await f.store.ready(f.identity));await assert.rejects(service.perform(id,false));assert.equal(writes,0);
  const result=JSON.stringify({operationId:uuid(),status:'applied',current:true,result:{revisionEpoch:'1',version:'1',contentState:'owned',replyState:'none'}});
  operation.state='completed';operation.result=result;await f.rows("UPDATE self_operations SET state='completed',payload=? WHERE id=?",[JSON.stringify(operation),id]);
  await assert.rejects(service.perform(id,true));assert.equal(writes,0);
}));
test('S2 cloud adoption compares local original plus lifecycle in the same SQLite transaction', () => withFixture(async f => {
  const {service}=await selfFixture(f,async()=>undefined);const original=JSON.stringify({text:'new local',replyId:'',editId:''});await f.rows('INSERT INTO local_state VALUES(?,?,?)',[f.identity.namespace,'draft:'+f.conversation.id,original]);
  await assert.rejects(service.adopt(f.conversation.id,'old local','remote',()=>true),/INPUT_CHANGED/);
  await assert.rejects(service.adopt(f.conversation.id,original,'remote',()=>false),/INPUT_CHANGED/);
  await service.adopt(f.conversation.id,original,'remote',()=>true);assert.equal(JSON.parse((await f.rows('SELECT value FROM local_state WHERE namespace=? AND key=?',[f.identity.namespace,'draft:'+f.conversation.id]))[0][0]).text,'remote');
}));
test('S2 stale visible membership rejects observation before creating an original write', () => withFixture(async f => {
  const {service,calls}=await selfFixture(f,async(route)=>{if(route==='/conversations/'+f.conversation.id)return {...f.conversation,membershipGeneration:'2'};});
  await assert.rejects(service.observe(f.conversation.id,[f.message.id],'1'),/QUALIFICATION_CHANGED/);assert.equal((await service.operations()).length,0);assert.equal(calls.filter(row=>row.method==='PUT').length,0);
}));
test('S2 personal controls and state labels cover all ten languages',()=>{
  const keys=['title','font','small','standard','large','draft','save','saveReply','clear','adopt','recovery','operations','continue','observe','read','views','all','hidden','archived','manual','pin','mute','unmute','readonly','help'];
  for(const language of ['zh-Hans','zh-Hant','bo','en','ja','fr','es','de','ru','ko']){for(const key of keys)assert.ok(SelfText.text(language,key).length>0);for(const state of ['pending','frozen','completed'])assert.ok(SelfText.state(language,state).length>0)}
});

test('committed message invalidation clears RAM before cleanup awaits and never fires for failed publication',()=>withFixture(async f=>{const order=[];f.ports.historyChanged=()=>order.push('invalidate');const cleanup=f.ports.cleanup;f.ports.cleanup=async()=>{order.push('cleanup');await cleanup();};await f.engine.run();assert.deepEqual(order,['cleanup','invalidate','cleanup']);f.state.beforeChanges=async()=>{f.state.current=false;};await assert.rejects(new SyncEngine(f.identity,f.ports).run());assert.equal(order.filter(item=>item==='invalidate').length,1);assert.deepEqual(order,['cleanup','invalidate','cleanup','cleanup']);}));
