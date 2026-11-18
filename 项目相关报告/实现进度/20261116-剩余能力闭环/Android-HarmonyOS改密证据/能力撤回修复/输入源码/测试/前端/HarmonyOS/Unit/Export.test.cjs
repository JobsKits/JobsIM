// Real shipped ArkTS under explicit host adapters, never Harmony device evidence.
const test = require('node:test');
const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');
const Module = require('node:module');
const { spawn } = require('node:child_process');
const work = process.env.JOBS_IM_EXPORT_WORK;
const { ExportRules } = require(path.join(work, 'ExportProtocol.js'));
const { ExportService } = require(path.join(work, 'ExportService.js'));
const { ExportStore } = require(path.join(work, 'ExportStore.js'));
const { ExportText } = require(path.join(work, 'ExportText.js'));
const { ApiFailure } = require(path.join(work, 'Models.js'));
const uuid = () => crypto.randomUUID();
const digest = value => crypto.createHash('sha256').update(value).digest('hex');
const codec = { bytes: value => new Uint8Array(Buffer.from(value)), sha256: async bytes => digest(bytes) };
const id = uuid(), artifact = uuid();
function view(state = 'ready') { return JSON.stringify({ job: { id, artifactId: artifact, restoreEpoch: '2', buildGeneration: '9007199254740993', state, cleanupState: state === 'ready' ? 'published' : 'none', expiresAt: new Date(Date.now() + 3600000).toISOString() }, format: 'zip', scope: 'personal_current', coverage: ['messages'], exclusions: ['secrets'] }); }
function fixture() {
  const calls = [], rows = []; let active = true, enabled = true, committed = false, lose = false, failDisk = false;
  const physical = uuid(), deployment = uuid();
  const repository = { list: async () => structuredClone(rows), insert: async row => { if (failDisk) throw Error('disk fixture'); rows.push(structuredClone(row)); }, replace: async (before, after) => { const index = rows.findIndex(row => row.id === before.id); assert.equal(JSON.stringify(rows[index]), JSON.stringify(before)); rows[index] = structuredClone(after); } };
  const ports = { deployment, epoch: '2', physical, repository, codec, uuid, current: () => active, download: async () => { throw Error('not a network fixture'); }, request: async (route, method, body) => {
    calls.push({ route, method, body });
    if (route === '/bootstrap') return JSON.stringify({ deploymentId: deployment, restoreEpoch: '2', features: { chat_export: enabled } });
    if (route === '/me/data-export-results') { if (!committed) throw new ApiFailure(404, 'EXPORT_OPERATION_NOT_FOUND'); return view(); }
    if (route === '/me/data-exports') { assert.equal(rows.length, 1); assert.equal(rows[0].body, body); committed = true; if (lose) throw new ApiFailure(0, 'TRANSPORT_UNKNOWN'); return view(); }
    if (route.endsWith('/cancel')) return view('cancelled');
    if (route.endsWith('/download-ticket')) return JSON.stringify({ format: 'zip', downloadToken: 'synthetic-ticket-not-secret', expiresAt: new Date(Date.now() + 30000).toISOString() });
    return view();
  } };
  return { service: new ExportService(ports), ports, rows, calls, physical, get committed() { return committed; }, set lose(value) { lose = value; }, set active(value) { active = value; }, set enabled(value) { enabled = value; }, set failDisk(value) { failDisk = value; } };
}
test('job projection requires exact strings but never private HTTP fields', () => {
  const result = ExportRules.view(view(), '2'); assert.equal(result.generation, '9007199254740993'); assert.ok(ExportRules.ready(result));
  for (const mutate of [row => row.job.restoreEpoch = 2, row => row.job.id = uuid(), row => row.job.state = 'success', row => row.coverage = [9]]) { const raw = JSON.parse(view()); mutate(raw); assert.throws(() => ExportRules.view(JSON.stringify(raw), '2', id)); }
});
test('ten languages cover every native export label and identify this device', () => {
  assert.equal(Object.keys(ExportText.rows).length, 10);
  for (const row of Object.values(ExportText.rows)) { assert.equal(row.length, ExportText.keys.length); assert.ok(row.every(value => value.trim())); }
  assert.match(ExportText.text('en', 'exportLocalList'), /device/); assert.match(ExportText.text('en', 'exportLimit'), /HarmonyOS/);
});
test('create persists original body before first POST and successful loss uses lookup only', async () => {
  const f = fixture(); f.lose = true; await assert.rejects(f.service.create(), error => error.code === 'TRANSPORT_UNKNOWN');
  const original = f.rows[0].body; assert.equal(f.rows[0].job, '');
  const result = await f.service.resume(f.rows[0].id, false); assert.equal(result.id, id); assert.equal(f.rows[0].job, id);
  assert.equal(f.calls.filter(row => row.route === '/me/data-exports').length, 1); assert.ok(f.calls.filter(row => row.route === '/me/data-export-results').every(row => row.body === original));
});
test('unknown fresh404 retains intent and only explicit same body can first create', async () => {
  const f = fixture(); const raw = JSON.stringify({ requestId: uuid(), restoreEpoch: '2' });
  f.rows.push({ id: JSON.parse(raw).requestId, body: raw, hash: digest(raw), physical: f.physical, created: new Date().toISOString(), job: '', cancelBody: '', cancelHash: '' });
  await assert.rejects(f.service.resume(f.rows[0].id, false), /EXPORT_RESULT_NOT_FOUND/); assert.equal(f.committed, false);
  await f.service.resume(f.rows[0].id, true); assert.equal(f.calls.find(row => row.route === '/me/data-exports').body, raw);
});
test('lookup wrong404 and transport failure never become creating POST', async () => {
  for (const failure of [new ApiFailure(404, 'NOT_FOUND'), new ApiFailure(503, 'DEPENDENCY_UNKNOWN')]) {
    const f = fixture(); const raw = JSON.stringify({ requestId: uuid(), restoreEpoch: '2' }); f.rows.push({ id: JSON.parse(raw).requestId, body: raw, hash: digest(raw), physical: f.physical, created: new Date().toISOString(), job: '', cancelBody: '', cancelHash: '' });
    const original = f.ports.request; f.ports.request = (route, method, body) => route.endsWith('results') ? Promise.reject(failure) : original(route, method, body);
    await assert.rejects(f.service.resume(f.rows[0].id, true)); assert.equal(f.committed, false);
  }
});
test('disk persistence failure sends zero creating request', async () => { const f = fixture(); f.failDisk = true; await assert.rejects(f.service.create()); assert.equal(f.calls.filter(row => row.route === '/me/data-exports').length, 0); });
test('capability withdrawal permits lookup/status/cancel but denies new create and delivery', async () => {
  const f = fixture(); await f.service.create(); f.enabled = false;
  await f.service.resume(f.rows[0].id, false); await f.service.status(f.rows[0].id); await f.service.cancel(f.rows[0].id);
  const first = f.rows[0].cancelBody; await f.service.cancel(f.rows[0].id); assert.equal(f.rows[0].cancelBody, first); assert.deepEqual(Object.keys(JSON.parse(first)), ['requestId']);
  await assert.rejects(f.service.create(), /EXPORT_CAPABILITY_REQUIRED/); await assert.rejects(f.service.delivery(f.rows[0].id), /EXPORT_CAPABILITY_REQUIRED/);
});
test('current family retirement blocks even late successful create from local publication', async () => {
  const f = fixture(); const original = f.ports.request;
  f.ports.request = async (route, method, body) => { const result = await original(route, method, body); if (route === '/me/data-exports') f.active = false; return result; };
  await assert.rejects(f.service.create(), /EXPORT_CONTEXT_CHANGED/); assert.equal(f.rows[0].job, '');
});
test('mutated body/hash and secret-bearing local records fail before any HTTP', async () => {
  const f = fixture(); await f.service.create(); f.calls.length = 0; f.rows[0].body = f.rows[0].body.replace('"2"', '"3"');
  await assert.rejects(f.service.status(f.rows[0].id)); assert.equal(f.calls.length, 1); // status first checks public capability, never owner data.
  const row = { ...f.rows[0], refreshToken: 'synthetic-forbidden' }; assert.throws(() => ExportRules.original(row, '2', f.physical));
});
test('tickets stay ephemeral and completed job404 never re-creates a task', async () => {
  const f = fixture(); await f.service.create(); const body = JSON.stringify(f.rows); f.ports.download = async (job, token) => ({ job, name: 'personal.zip', path: '/synthetic/private.zip', size: 100, digest: digest('synthetic') });
  await f.service.delivery(f.rows[0].id); assert.equal(JSON.stringify(f.rows), body); assert.ok(!body.includes('ticket'));
  f.ports.request = async route => { if (route === '/bootstrap') return JSON.stringify({ deploymentId: f.ports.deployment, restoreEpoch: '2' }); throw new ApiFailure(404, 'EXPORT_OPERATION_NOT_FOUND'); };
  await assert.rejects(f.service.resume(f.rows[0].id, true), /EXPORT_RESULT_NOT_FOUND/);
});
function archive() {
  const local = Buffer.alloc(31), directory = Buffer.alloc(47), end = Buffer.alloc(22);
  local.writeUInt32LE(0x04034b50); directory.writeUInt32LE(0x02014b50); directory.writeUInt16LE(1, 28); directory[46] = 97;
  end.writeUInt32LE(0x06054b50); end.writeUInt16LE(1, 8); end.writeUInt16LE(1, 10); end.writeUInt32LE(47, 12); end.writeUInt32LE(31, 16);
  return Buffer.concat([local, directory, end]);
}
test('bounded ZIP slices reject truncated200, tail garbage and forged central record', () => {
  const data = archive(); const read = buffer => (offset, size) => new Uint8Array(buffer.subarray(offset, offset + size));
  ExportRules.zip(data.length, read(data));
  for (const bad of [data.subarray(0, data.length - 1), Buffer.concat([data, Buffer.from('x')]), Buffer.from(data)]) {
    if (bad.length === data.length) bad[31] = 0;
    assert.throws(() => ExportRules.zip(bad.length, read(bad)), /EXPORT_FILE_INVALID/);
  }
  assert.throws(() => ExportRules.zip(ExportRules.limit + 1, () => { throw Error('must not read'); }), /EXPORT_FILE_LIMIT/);
});
test('server filename is safe, localized and ZIP only', () => {
  assert.equal(ExportRules.fileName("attachment; filename*=UTF-8''%E6%88%91%E7%9A%84.zip"), '我的.zip');
  for (const name of ['../private.zip', 'a\\private.zip', 'x\u202ezip.zip', 'a.txt', ' a.zip']) assert.throws(() => ExportRules.fileName('attachment; filename="' + name + '"'));
  assert.throws(() => ExportRules.fileName('attachment; filename="a.zip"; filename="b.zip"'));
});

class Bridge {
  constructor(file) { this.next = 0; this.pending = new Map(); this.buffer = ''; this.tail = Promise.resolve(); this.process = spawn('python3', ['-u', process.env.JOBS_IM_EXPORT_SQLITE, file], { stdio: ['pipe', 'pipe', 'pipe'] }); this.process.stdout.on('data', bytes => { this.buffer += bytes; while (this.buffer.includes('\n')) { const at = this.buffer.indexOf('\n'), row = JSON.parse(this.buffer.slice(0, at)); this.buffer = this.buffer.slice(at + 1); const p = this.pending.get(row.id); this.pending.delete(row.id); row.error ? p.reject(Error(row.error)) : p.resolve(row.rows); } }); }
  sql(sql, values = []) { const work = this.tail.then(() => new Promise((resolve, reject) => { const id = ++this.next; this.pending.set(id, { resolve, reject }); this.process.stdin.write(JSON.stringify({ id, sql, values }) + '\n'); })); this.tail = work.catch(() => {}); return work; }
  async executeSql(sql, values = []) { await this.sql(sql, values); }
  async querySql(sql, values = []) { const rows = await this.sql(sql, values); let at = -1; return { goToNextRow: () => ++at < rows.length, getString: column => String(rows[at][column]), close() {} }; }
  beginTransaction() { this.sql('BEGIN IMMEDIATE'); } commit() { this.sql('COMMIT'); } rollBack() { this.sql('ROLLBACK'); }
  async close() { await this.tail; this.process.stdin.end(); await new Promise(resolve => this.process.once('exit', resolve)); }
}
test('shipped repository persists original across reopen, enforces CAS and physical UUID', async () => {
  const bridge = new Bridge(path.join(work, 'Export-adapter.db')); let tail = Promise.resolve(); const physical = uuid();
  const local = { syncAccess(action) { const result = tail.then(() => action(bridge)); tail = result.catch(() => {}); return result; } };
  try {
    await bridge.executeSql('CREATE TABLE local_state(namespace TEXT,key TEXT,value TEXT,PRIMARY KEY(namespace,key))'); await bridge.executeSql('INSERT INTO local_state VALUES(?,?,?)', ['__sync_store__', 'physical', physical]);
    const store = new ExportStore(local, 'owned-family', '2', physical, codec, () => true); await store.open();
    const body = JSON.stringify({ requestId: uuid(), restoreEpoch: '2' }); const row = { id: JSON.parse(body).requestId, body, hash: digest(body), created: new Date().toISOString(), physical, job: '', cancelBody: '', cancelHash: '' };
    await store.insert(row); const reopened = new ExportStore(local, 'owned-family', '2', physical, codec, () => true); assert.deepEqual(await reopened.list(), [row]);
    const next = { ...row, job: id }; await reopened.replace(row, next); await assert.rejects(store.replace(row, next), /EXPORT_ORIGINAL_CHANGED/);
    assert.deepEqual(await new ExportStore(local, 'other-family', '2', physical, codec, () => true).list(), []);
    await bridge.executeSql("UPDATE local_state SET value=? WHERE key='physical'", [uuid()]); await assert.rejects(reopened.list(), /EXPORT_STORE_CHANGED/);
  } finally { await bridge.close(); }
});

function apiFixture() {
  let active = true, mode = 'success', refresh = 0; const calls = [], events = []; const bytes = archive();
  class Request { constructor(url, method, headers, body) { Object.assign(this, { url, method, headers, body }); } }
  const rcp = { Request, createSession: configuration => ({ close() { events.push('close'); }, async fetch(request) { calls.push({ request, configuration });
    if (mode === 'expired' && calls.length === 1) { const error = Buffer.from('{"error":{"code":"ACCESS_EXPIRED"}}'); request.destination(error.buffer.slice(error.byteOffset, error.byteOffset + error.length)); return { statusCode: 401, headers: {} }; }
    const headers = { 'content-type': 'application/zip', 'content-disposition': 'attachment; filename="personal.zip"', 'content-length': String(mode === 'limit' ? ExportRules.limit + 1 : bytes.length) };
    request.configuration.tracing.httpEventsHandler.onHeaderReceive(headers);
    if (mode === 'retire') active = false;
    request.destination(bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.length)); return { statusCode: 200, headers };
  } }) };
  const file = path.join(work, 'Api.js'), loaded = new Module(file); loaded.filename = file;
  loaded.require = name => name === '@kit.RemoteCommunicationKit' ? { rcp } : name === '@ohos.file.fs' ? { default: {} } : require(path.resolve(work, name)); loaded._compile(fs.readFileSync(file, 'utf8'), file);
  let body = Buffer.alloc(0); const sink = { reset() { events.push('reset'); body = Buffer.alloc(0); }, write(chunk) { events.push('write'); body = Buffer.concat([body, Buffer.from(chunk)]); }, errorText() { return body.toString(); }, async finish(job, name, size) { events.push('finish'); assert.equal(size, bytes.length); return { job, name }; }, dispose() { events.push('dispose'); } };
  const hooks = { stamp: () => 'synthetic-family', before: async () => 'synthetic-at', refresh: async () => { refresh++; return 'synthetic-new-at'; }, rejected: async () => {} };
  return { api: new loaded.exports.Api('https://strict.invalid', '', '/private/ca.pem', hooks), sink, events, calls, current: () => active, set mode(value) { mode = value; }, get refresh() { return refresh; } };
}
test('actual API streams to sink with CA, no redirects and RAM-only ticket header', async () => {
  const f = apiFixture(); await f.api.dataExport(id, 'synthetic-ticket-string', f.sink, f.current);
  const { request, configuration } = f.calls[0]; assert.ok(!request.url.includes('ticket')); assert.equal(request.headers['X-JobsIM-Download-Token'], 'synthetic-ticket-string'); assert.equal(configuration.requestConfiguration.transfer.autoRedirect, false); assert.deepEqual(configuration.requestConfiguration.security.remoteValidation, { filePath: '/private/ca.pem' }); assert.ok(f.events.includes('finish'));
});
test('oversized headers and identity retirement close stream without finish', async () => {
  for (const mode of ['limit', 'retire']) { const f = apiFixture(); f.mode = mode; await assert.rejects(f.api.dataExport(id, 'synthetic-ticket-string', f.sink, f.current)); assert.ok(f.events.includes('dispose')); assert.ok(!f.events.includes('finish')); }
});
test('expired download coordinates one refresh and resets partial error bytes', async () => {
  const f = apiFixture(); f.mode = 'expired'; await f.api.dataExport(id, 'synthetic-ticket-string', f.sink, f.current);
  assert.equal(f.refresh, 1); assert.equal(f.events.filter(value => value === 'reset').length, 2); assert.equal(f.events.filter(value => value === 'finish').length, 1);
  assert.equal(f.calls[0].request.headers['X-JobsIM-Download-Token'], f.calls[1].request.headers['X-JobsIM-Download-Token']);
});

function fileFixture() {
  const root = fs.mkdtempSync(path.join(work, 'Private-files-')), events = [], offsets = new Map(); let selected = [], active = true;
  const normalize = value => value.startsWith('file://') ? require('node:url').fileURLToPath(value) : value;
  const nativeFS = { OpenMode: { CREATE: 64, READ_WRITE: 2, TRUNC: 512, READ_ONLY: 0, WRITE_ONLY: 1 }, WhenceType: { SEEK_SET: 0 },
    accessSync: value => fs.existsSync(normalize(value)), mkdirSync: value => fs.mkdirSync(value), listFileSync: value => fs.readdirSync(value), unlinkSync: value => { events.push('unlink'); fs.unlinkSync(normalize(value)); }, renameSync: (a, b) => { events.push('rename'); fs.renameSync(a, b); },
    openSync: (value, flags) => { const fd = fs.openSync(normalize(value), flags, 0o600); offsets.set(fd, 0); return { fd }; }, closeSync: value => { const fd = typeof value === 'number' ? value : value.fd; fs.closeSync(fd); offsets.delete(fd); },
    lseek: (fd, offset) => { offsets.set(fd, offset); return offset; }, statSync: fd => fs.fstatSync(fd),
    readSync: (fd, data) => { const count = fs.readSync(fd, new Uint8Array(data), 0, data.byteLength, offsets.get(fd)); offsets.set(fd, offsets.get(fd) + count); return count; },
    writeSync: (fd, data) => { const count = fs.writeSync(fd, new Uint8Array(data), 0, data.byteLength, offsets.get(fd)); offsets.set(fd, offsets.get(fd) + count); events.push('write'); return count; },
    fsyncSync: fd => { events.push(fs.fstatSync(fd).isDirectory() ? 'directory-fsync' : 'file-fsync'); fs.fsyncSync(fd); } };
  const util = { generateRandomUUID: uuid, TextDecoder: { create: (...args) => { const decoder = new TextDecoder(...args); return { decodeToString: bytes => decoder.decode(bytes) }; } } };
  class SaveOptions {} class Selector { async save() { events.push('picker'); return selected; } }
  const cryptoFramework = { createMd: () => { const hasher = crypto.createHash('sha256'); return { async update({ data }) { hasher.update(data); }, async digest() { return { data: new Uint8Array(hasher.digest()) }; } }; } };
  const file = path.join(work, 'ExportFiles.js'), loaded = new Module(file); loaded.filename = file;
  loaded.require = name => name === '@ohos.file.fs' ? { default: nativeFS } : name === '@ohos.util' ? { default: util } : name === '@ohos.file.picker' ? { default: { DocumentViewPicker: Selector, DocumentSaveOptions: SaveOptions } } : name === '@ohos.security.cryptoFramework' ? { default: cryptoFramework } : require(path.resolve(work, name)); loaded._compile(fs.readFileSync(file, 'utf8'), loaded.filename);
  const context = { cacheDir: root }; const files = new loaded.exports.ExportFiles(context, () => active);
  return { files, context, root, events, set selected(value) { selected = value; }, set active(value) { active = value; } };
}
test('native file implementation fsyncs file and directory before chooser and writes exact ZIP', async () => {
  const f = fileFixture(); const bytes = archive(); f.files.reset(); f.files.write(bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.length));
  const prepared = await f.files.finish(id, 'personal.zip', bytes.length); assert.equal(prepared.digest, digest(bytes));
  assert.deepEqual(f.events.slice(-3), ['file-fsync', 'rename', 'directory-fsync']);
  const target = path.join(f.root, 'chosen.zip'); fs.writeFileSync(target, ''); f.selected = ['file://' + target];
  assert.equal(await f.files.save(f.context, prepared, async () => { f.events.push('reauthorize'); }), true);
  assert.ok(f.events.indexOf('directory-fsync') < f.events.indexOf('picker')); assert.ok(f.events.indexOf('picker') < f.events.indexOf('reauthorize')); assert.deepEqual(fs.readFileSync(target), bytes); f.files.dispose(); assert.equal(fs.existsSync(prepared.path), false);
});
test('cancelled native picker is not saved; late identity failure cleans its new empty destination', async () => {
  const f = fileFixture(), bytes = archive(); f.files.reset(); f.files.write(bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.length)); const prepared = await f.files.finish(id, 'personal.zip', bytes.length);
  assert.equal(await f.files.save(f.context, prepared, async () => { throw Error('must not authorize cancelled picker'); }), false);
  const target = path.join(f.root, 'cancelled.zip'); fs.writeFileSync(target, ''); f.selected = ['file://' + target];
  await assert.rejects(f.files.save(f.context, prepared, async () => { f.active = false; })); assert.equal(fs.existsSync(target), false); f.files.dispose();
});
