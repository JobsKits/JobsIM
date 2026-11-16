const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const Module = require('node:module');
const { ApiFailure } = require(process.env.JOBS_IM_HARMONY_HOST_MODELS);

function fixture() {
  const calls = [], opened = [], closed = [];
  let stamp = 'original-family', token = 'original-at', refreshes = 0, rejections = 0;
  let respond = async request => ({ statusCode: 200, headers: {}, body: new ArrayBuffer(1), toString: () => JSON.stringify({ ok: true }) });
  class Request { constructor(url, method, headers, body) { Object.assign(this, { url, method, headers, body }); } }
  const rcp = { Request, MultipartForm: class { constructor(fields) { this.fields = fields; } }, createSession: configuration => ({
    fetch: async request => { calls.push({ request, configuration }); return respond(request); }, close: () => closed.push(true)
  }) };
  const file = { OpenMode: { READ_ONLY: 0 }, openSync: path => { opened.push(path); return { fd: 1 }; }, statSync: () => ({ size: 4 }), readSync: (fd, buffer) => buffer.byteLength, closeSync: () => {} };
  const compiled = process.env.JOBS_IM_HARMONY_HOST_API;
  const module = new Module(compiled);
  module.filename = compiled;
  module.require = id => id === '@kit.RemoteCommunicationKit' ? { rcp } : id === '@ohos.file.fs' ? { default: file } : require(id === './Models' ? process.env.JOBS_IM_HARMONY_HOST_MODELS : id);
  module._compile(fs.readFileSync(compiled, 'utf8'), compiled);
  const hooks = { stamp: () => stamp, before: async original => { assert.equal(original, stamp); return token; },
    refresh: async old => { assert.equal(old, 'original-at'); refreshes++; token = 'rotated-at'; return token; }, rejected: async () => { rejections++; } };
  const api = new module.exports.Api('https://instance.example.test', 'unused-at', '/private/instance-ca.pem', hooks);
  return { api, calls, opened, closed, get refreshes() { return refreshes; }, get rejections() { return rejections; },
    set respond(value) { respond = value; }, set stamp(value) { stamp = value; } };
}
function expired() { return { statusCode: 401, toString: () => JSON.stringify({ error: { code: 'ACCESS_EXPIRED' } }) }; }

test('business JSON, upload, attachment and avatar share one expiry coordination path', async () => {
  for (const run of [api => api.request('/me'), api => api.upload('/private/file'), api => api.download('/messages/original/attachment'), api => api.avatar('/private/avatar', 'same-request', '7')]) {
    const f = fixture();
    f.respond = async request => request.headers.Authorization === 'Bearer original-at' ? expired() : ({ statusCode: 200, headers: { 'content-type': 'application/octet-stream' }, body: new ArrayBuffer(3), toString: () => '{"ok":true}' });
    await run(f.api);
    assert.equal(f.refreshes, 1); assert.equal(f.calls.length, 2); assert.equal(f.closed.length, 2);
    assert.equal(f.calls[1].request.headers.Authorization, 'Bearer rotated-at');
    assert.equal(f.calls[0].configuration.requestConfiguration.transfer.autoRedirect, false);
    assert.deepEqual(f.calls[0].configuration.requestConfiguration.security.remoteValidation, { filePath: '/private/instance-ca.pem' });
    if (f.calls[0].request.body?.fields?.requestId) assert.equal(f.calls[1].request.body.fields.requestId, 'same-request');
  }
});
test('retired API binding stops before file read and network', async () => {
  const f = fixture(); f.stamp = 'new-device';
  await assert.rejects(f.api.upload('/private/file'));
  assert.equal(f.opened.length, 0); assert.equal(f.calls.length, 0);
});
test('late successful business response cannot cross a new session namespace', async () => {
  const f = fixture(); f.respond = async () => { f.stamp = 'new-device'; return { statusCode: 200, toString: () => '{"old":true}' }; };
  await assert.rejects(f.api.request('/me'), /SESSION_SCOPE_CHANGED/); assert.equal(f.refreshes, 0);
});
test('transport unknown never consumes a refresh token or silently repeats a write', async () => {
  const f = fixture(); f.respond = async () => { throw Error('transport fixture'); };
  await assert.rejects(f.api.request('/messages', 'POST', '{"clientMsgId":"original"}'), error => error instanceof ApiFailure && error.code === 'TRANSPORT_UNKNOWN');
  assert.equal(f.calls.length, 1); assert.equal(f.refreshes, 0); assert.equal(f.rejections, 0);
});
test('public initial authentication never attaches an existing access token', async () => {
  const f = fixture(); await f.api.request('/auth/reauthenticate', 'POST', '{}');
  assert.equal(f.calls.length, 1); assert.equal(f.calls[0].request.headers.Authorization, undefined); assert.equal(f.refreshes, 0);
});
