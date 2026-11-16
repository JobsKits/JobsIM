// Created by Jobs on 2026年11月15日，星期日.
'use strict';
const fs = require('node:fs');
const path = require('node:path');
const os = require('node:os');
const https = require('node:https');
const crypto = require('node:crypto');
const assert = require('node:assert/strict');
const { spawn } = require('node:child_process');
const binary = process.env.JOBSIM_CLI_BINARY;
const ca = process.env.JOBSIM_CA_FILE;
const server = process.env.JOBSIM_SERVER_URL || 'https://127.0.0.1:28443';
if (!binary || !ca) throw Error('Provide JOBSIM_CLI_BINARY and JOBSIM_CA_FILE');
const origin = new URL(server);
if (origin.protocol !== 'https:' || origin.username || origin.password || origin.search || origin.hash) throw Error('Verified HTTPS instance required');
const trust = new https.Agent({ ca: fs.readFileSync(ca) });
function api(route, method = 'GET', body, token) {
  return new Promise((resolve, reject) => {
    const request = https.request(server + '/api' + route, { method, agent: trust, headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: 'Bearer ' + token } : {}) } }, response => {
      let text = ''; response.on('data', chunk => { text += chunk; });
      response.on('end', () => { try { const data = JSON.parse(text || '{}'); if (response.statusCode >= 400) reject(Error('HTTP ' + response.statusCode + ' ' + data.error?.code)); else resolve(data); } catch { reject(Error('Invalid API response')); } });
    });
    request.setTimeout(20000, () => request.destroy(Error('TIMEOUT')));
    request.on('error', error => reject(Error(error.code || 'NETWORK')));
    request.end(body === undefined ? undefined : JSON.stringify(body));
  });
}
class CLI {
  constructor(mode, user, password, directory) {
    this.output = ''; this.error = ''; this.waiter = null;
    this.process = spawn(binary, ['--server', server, '--ca', ca, '--data-dir', directory, mode], { stdio: ['pipe', 'pipe', 'pipe'] });
    this.process.stdout.on('data', chunk => { this.output += chunk; this.flush(); });
    this.process.stderr.on('data', chunk => { this.error += chunk; });
    this.closed = new Promise((resolve, reject) => {
      this.process.once('error', error => { this.waiter?.reject(Error(error.code || 'CLI_START')); reject(error); });
      this.process.once('exit', code => { this.waiter?.reject(Error('CLI exited before prompt')); this.waiter = null; resolve(code); });
    });
    this.ready = this.prompt(); this.process.stdin.write(user + '\n' + password + '\n');
  }
  flush() { if (!this.waiter) return; const at = this.output.indexOf('jobsim> '); if (at < 0) return; const answer = this.output.slice(0, at); this.output = this.output.slice(at + 8); const waiter = this.waiter; this.waiter = null; clearTimeout(waiter.timer); waiter.resolve(answer); }
  prompt() { return new Promise((resolve, reject) => { this.waiter = { resolve, reject, timer: setTimeout(() => { this.waiter = null; reject(Error('CLI prompt timeout')); }, 25000) }; this.flush(); }); }
  async command(line) { const previous = this.error.length; const next = this.prompt(); this.process.stdin.write(line + '\n'); const raw = await next; await new Promise(resolve => setTimeout(resolve, 25)); if (this.error.length !== previous) throw Error('CLI rejected command ' + line.split(' ')[0]); return raw.trim() ? JSON.parse(raw.trim()) : null; }
  async stop() { if (this.process.exitCode !== null) return; this.process.stdin.end('quit\n'); assert.equal(await this.closed, 0); }
}
function noProof(value) {
  if (Array.isArray(value)) { for (const item of value) noProof(item); }
  else if (value && typeof value === 'object') { assert.equal(Object.hasOwn(value, 'forwardAdmission'), false); for (const item of Object.values(value)) noProof(item); }
}
(async () => {
  const boot = await api('/bootstrap');
  for (const flag of ['send_admission', 'role_admission', 'mentions', 'announcements']) assert.equal(boot.features[flag], true);
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'jobsim-cli-collaboration-'));
  fs.chmodSync(directory, 0o700);
  const suffix = Date.now().toString(36) + crypto.randomBytes(3).toString('hex');
  const username = 'cli_collab_a_' + suffix;
  const password = 'Private CLI collaboration ' + crypto.randomUUID();
  const peer = await api('/auth/register', 'POST', { username: 'cli_collab_b_' + suffix, password, requestId: crypto.randomUUID(), deviceName: 'CLI collaboration peer' });
  const clients = []; let self;
  try {
    const cli = new CLI('register', username, password, directory); clients.push(cli); await cli.ready;
    self = await api('/auth/login', 'POST', { username, password, requestId: crypto.randomUUID(), deviceName: 'CLI collaboration API fixture' });
    const me = self.user.id;
    await cli.command('request ' + peer.user.id);
    const invitation = (await api('/contacts', 'GET', undefined, peer.token)).requests.find(item => item.state === 'pending' && item.requesterId === me);
    assert.ok(invitation); await api('/contacts/requests/' + invitation.id + '/accept', 'POST', {}, peer.token);
    const group = await cli.command('group CLI collaboration group | ' + peer.user.id);
    const snapshot = await api('/conversations/' + group.id, 'GET', undefined, self.token);
    const label = 'CLI-page-' + suffix; const sources = [];
    for (let i = 0; i < 51; i++) {
      const message = await api('/conversations/' + group.id + '/messages', 'POST', { clientMsgId: crypto.randomUUID(), restoreEpoch: boot.restoreEpoch, sendAdmission: snapshot.sendAdmission, type: 'text', text: label + ' ' + i }, self.token);
      sources.push(message); await api('/messages/' + message.id + '/favorite', 'PUT', undefined, self.token);
    }
    let page = await cli.command('search ' + label + ' | ' + group.id); assert.equal(page.items.length, 50); assert.equal(page.hasMore, true); noProof(page);
    let tail = await cli.command('more'); assert.equal(tail.items.length, 1); assert.equal(tail.hasMore, false); noProof(tail);
    const source = sources[0]; let current = await api('/messages/' + source.id, 'GET', undefined, self.token);
    current = await api('/messages/' + source.id, 'PATCH', { text: label + ' updated exact original', expectedVersion: current.version, requestId: crypto.randomUUID() }, self.token);
    const exact = await cli.command('open-result ' + source.id); assert.equal(exact.message.text, label + ' updated exact original'); assert.equal(exact.message.contentVersion, current.contentVersion); noProof(exact);
    page = await cli.command('favorites ' + group.id); tail = await cli.command('more'); assert.equal(page.items.length, 50); assert.equal(tail.items.length, 1); noProof(page); noProof(tail);
    const directed = await cli.command('mention ' + group.id + ' ' + peer.user.id + ' | CLI structured member mention');
    assert.deepEqual(directed.mentionedUserIds, [peer.user.id]); assert.equal((await api('/messages/' + directed.id, 'GET', undefined, peer.token)).mentionedMe, true);
    const everyone = await cli.command('mention ' + group.id + ' all | CLI owner all mention'); assert.equal(everyone.mentionAll, true); assert.equal((await api('/messages/' + everyone.id, 'GET', undefined, peer.token)).mentionedMe, true);
    assert.equal((await cli.command('announcement ' + group.id)).state, 'empty');
    let announced = await cli.command('announce ' + group.id + ' ' + source.id + ' ' + current.contentVersion + ' none'); assert.equal(announced.currentAnnouncement.state, 'active');
    const oldOperation = announced.operationId; const oldVersion = announced.currentAnnouncement.version;
    await cli.command('unfavorite ' + source.id);
    assert.equal((await cli.command('announcement ' + group.id)).state, 'active');
    announced = await cli.command('announce ' + group.id + ' ' + source.id + ' ' + current.contentVersion + ' 1d'); assert.ok(announced.currentAnnouncement.expiresAt); assert.notEqual(announced.currentAnnouncement.version, oldVersion);
    const newerVersion = announced.currentAnnouncement.version;
    const oldFact = await cli.command('resume-announcement ' + oldOperation); assert.equal(oldFact.originalFact.current, false); assert.equal(oldFact.currentAnnouncement.version, newerVersion);
    const operations = await cli.command('announcement-operations'); assert.ok(operations.every(item => !JSON.stringify(item.payload).includes(label))); assert.ok(operations.every(item => !JSON.stringify(item.result).includes(label)));
    current = await api('/messages/' + source.id, 'GET', undefined, self.token);
    await api('/messages/' + source.id, 'PATCH', { text: label + ' invalidated source', expectedVersion: current.version, requestId: crypto.randomUUID() }, self.token);
    assert.equal((await cli.command('announcement ' + group.id)).state, 'unavailable');
    const cleared = await cli.command('clear-announcement ' + group.id); assert.equal(cleared.currentAnnouncement.state, 'empty');
    const before = await cli.command('outbox');
    await cli.command('transfer ' + group.id + ' ' + peer.user.id);
    await assert.rejects(() => cli.command('mention ' + group.id + ' all | old owner cannot mention all'));
    await assert.rejects(() => cli.command('announce ' + group.id + ' ' + source.id + ' ' + source.contentVersion + ' none'));
    assert.equal(Object.keys(await cli.command('outbox')).length, Object.keys(before).length);
    const factAfterRole = await cli.command('resume-announcement ' + oldOperation); assert.equal(factAfterRole.currentAnnouncement.canManage, false);
    page = await cli.command('favorites ' + group.id); assert.ok(page.items.length > 0); const oldSource = page.items[0];
    const membership = await api('/conversations/' + group.id, 'GET', undefined, self.token);
    await api('/conversations/' + group.id + '/members/' + me + '?membershipGeneration=' + membership.membershipGeneration, 'DELETE', undefined, peer.token);
    await api('/conversations/' + group.id + '/members', 'POST', { userId: me }, peer.token);
    await assert.rejects(() => cli.command('open-result ' + oldSource.id)); await assert.rejects(() => cli.command('more'));
    await cli.stop();
    const reopened = new CLI('login', username, password, directory); clients.push(reopened); await reopened.ready;
    const kept = await reopened.command('announcement-operations'); assert.ok(kept.some(item => item.operationId === oldOperation && item.state === 'completed')); noProof(kept); await reopened.stop();
    const report = { at: new Date().toISOString(), status: 'passed', binarySHA256: crypto.createHash('sha256').update(fs.readFileSync(binary)).digest('hex'), checks: [
      'real server search and favorites 50+1 opaque pages; ordinary output strips reader proof',
      'exact source GET reflects later edit without trusting old list body',
      'current member and owner-all structured mentions reach actual recipient mentionedMe',
      'announcement source uses contentVersion independent from favorite version',
      'publish none/1d, old operation current=false preserves newer current reference, source edit unavailable and clear',
      'owner role loss rejects new mention-all and announcement before enqueue',
      'completed announcement facts survive process reopen without storing source body',
      'source membership round trip rejects old selected result, clears paging session'
    ] };
    const reportDirectory = process.env.JOBSIM_CLI_REPORT_DIR || path.resolve(__dirname, '../报告/20261115-搜索协作'); fs.mkdirSync(reportDirectory, { recursive: true }); fs.writeFileSync(path.join(reportDirectory, 'CollaborationHTTPS结果.json'), JSON.stringify(report, null, 2)); console.log(JSON.stringify(report));
  } finally {
    for (const cli of clients) { try { await cli.stop(); } catch { cli.process.kill('SIGTERM'); } }
    if (self) await api('/auth/logout', 'POST', {}, self.token).catch(() => {});
    await api('/auth/logout', 'POST', {}, peer.token).catch(() => {});
    trust.destroy();
  }
})().catch(error => { console.error(error.message); process.exitCode = 1; });
